package itests

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/n-r-w/itestkit"
	"github.com/n-r-w/itestkit/docs/itestkit/examples/real/service"
	itestkitbufconn "github.com/n-r-w/itestkit/grpc/bufconn"
	grpcpkg "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

const realBufConnSize = 1024 * 1024

// caseHarness stores dependencies of one integration case.
type caseHarness struct {
	repository    *service.InMemoryOrderRepository
	apiClient     grpc_health_v1.HealthClient
	externalState *externalMockState
}

// newCaseHarness starts an environment "like in a real service":
// 1) in-memory DB;
// 2) external gRPC mock;
// 3) internal gRPC API that calls the external API.
func newCaseHarness(t *testing.T) *caseHarness {
	repository := service.NewInMemoryOrderRepository()
	externalState := newExternalMockState()

	externalClient := itestkitbufconn.NewClient(
		t,
		func(grpcServer *grpcpkg.Server) {
			grpc_health_v1.RegisterHealthServer(
				grpcServer,
				&externalHealthServer{
					UnimplementedHealthServer: grpc_health_v1.UnimplementedHealthServer{},
					state:                     externalState,
				},
			)
		},
		grpc_health_v1.NewHealthClient,
		itestkitbufconn.WithBufSize(realBufConnSize),
	)

	apiClient := itestkitbufconn.NewClient(
		t,
		func(grpcServer *grpcpkg.Server) {
			grpc_health_v1.RegisterHealthServer(grpcServer, service.NewAPIServer(repository, externalClient))
		},
		grpc_health_v1.NewHealthClient,
		itestkitbufconn.WithBufSize(realBufConnSize),
	)

	return &caseHarness{
		repository:    repository,
		apiClient:     apiClient,
		externalState: externalState,
	}
}

// externalPlannedCall describes one expected outbound external API call.
type externalPlannedCall struct {
	ExpectedService string
	StubStatus      grpc_health_v1.HealthCheckResponse_ServingStatus
}

// externalSnapshot stores an immutable snapshot of external mock state.
type externalSnapshot struct {
	Planned bool
	Plan    []externalPlannedCall
	Calls   []string
}

// externalMockState stores mutable state of the external mock.
type externalMockState struct {
	mu      sync.Mutex
	planned bool
	plan    []externalPlannedCall
	calls   []string
}

// newExternalMockState creates empty external mock state.
func newExternalMockState() *externalMockState {
	return &externalMockState{
		mu:      sync.Mutex{},
		planned: false,
		plan:    make([]externalPlannedCall, 0),
		calls:   make([]string, 0),
	}
}

// Plan stores external call plan and clears actual call log.
func (state *externalMockState) Plan(plannedCalls []externalPlannedCall) {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.planned = true
	state.plan = slices.Clone(plannedCalls)
	state.calls = state.calls[:0]
}

// ConsumeCall validates the next outbound call and returns prepared response.
func (state *externalMockState) ConsumeCall(
	serviceName string,
) (grpc_health_v1.HealthCheckResponse_ServingStatus, error) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if !state.planned {
		return grpc_health_v1.HealthCheckResponse_UNKNOWN, errors.New("external calls are not planned")
	}

	callIndex := len(state.calls)
	state.calls = append(state.calls, serviceName)
	if callIndex >= len(state.plan) {
		return grpc_health_v1.HealthCheckResponse_UNKNOWN, fmt.Errorf(
			"unexpected outbound call #%d: got %q, but no more calls are planned",
			callIndex+1,
			serviceName,
		)
	}

	plannedCall := state.plan[callIndex]
	if serviceName != plannedCall.ExpectedService {
		return grpc_health_v1.HealthCheckResponse_UNKNOWN, fmt.Errorf(
			"outbound call #%d mismatch: want %q got %q",
			callIndex+1,
			plannedCall.ExpectedService,
			serviceName,
		)
	}

	return plannedCall.StubStatus, nil
}

// Snapshot returns a consistent snapshot of mock state.
func (state *externalMockState) Snapshot() externalSnapshot {
	state.mu.Lock()
	defer state.mu.Unlock()

	return externalSnapshot{
		Planned: state.planned,
		Plan:    slices.Clone(state.plan),
		Calls:   slices.Clone(state.calls),
	}
}

// externalHealthServer simulates external gRPC API.
type externalHealthServer struct {
	grpc_health_v1.UnimplementedHealthServer
	state *externalMockState
}

// Check validates inbound outbound-request and returns a stub response.
func (server *externalHealthServer) Check(
	_ context.Context,
	request *grpc_health_v1.HealthCheckRequest,
) (*grpc_health_v1.HealthCheckResponse, error) {
	serviceName := strings.TrimSpace(request.GetService())
	if serviceName == "" {
		return nil, status.Error(codes.InvalidArgument, "service is required")
	}

	stubStatus, err := server.state.ConsumeCall(serviceName)
	if err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}

	return &grpc_health_v1.HealthCheckResponse{Status: stubStatus}, nil
}

// harnessFactory creates isolated harness per case.
type harnessFactory struct{}

var (
	_ itestkit.HarnessFactory[*caseHarness]                    = (*harnessFactory)(nil)
	_ itestkit.SuiteLifecycle[struct{}]                        = (*harnessFactory)(nil)
	_ itestkit.SuiteCaseHarnessFactory[struct{}, *caseHarness] = (*harnessFactory)(nil)
)

// New creates environment for one case.
func (harnessFactory) New(t *testing.T) *caseHarness {
	return newCaseHarness(t)
}

// SetupSuite prepares suite context. This example does not need shared context.
func (harnessFactory) SetupSuite(_ *testing.T) (struct{}, error) {
	return struct{}{}, nil
}

// TeardownSuite releases suite resources. There is nothing to release in this example.
func (harnessFactory) TeardownSuite(_ *testing.T, _ struct{}) error {
	return nil
}

// NewCaseHarness creates harness for a specific case.
func (factory harnessFactory) NewCaseHarness(t *testing.T, _ struct{}) *caseHarness {
	return factory.New(t)
}
