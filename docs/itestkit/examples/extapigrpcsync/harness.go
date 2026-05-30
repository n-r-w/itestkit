package extapigrpcsync

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/n-r-w/itestkit"
	itestkitbufconn "github.com/n-r-w/itestkit/grpc/bufconn"
	grpcpkg "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

const (
	bufConnSize           = 1024 * 1024
	serviceChainSeparator = "|"
)

// grpcSyncHarness stores case-level dependencies.
//
// Why this is a separate object:
// 1) all steps of one JSONC case must share the same mock state;
// 2) neighboring cases must not share calls and expectations.
type grpcSyncHarness struct {
	apiClient     grpc_health_v1.HealthClient
	externalState *externalMockState
}

// newGRPCSyncHarness starts two in-memory gRPC servers.
//
// Why two servers:
// 1) internal API server is the "service under test" called by the action step;
// 2) external mock server replaces the external API called by the internal server during processing.
// This mirrors the real production flow "inbound API -> outbound API" without network access.
func newGRPCSyncHarness(t *testing.T) *grpcSyncHarness {
	externalState := newExternalMockState()

	// External server accepts outbound calls and validates request against the prepare-step plan.
	externalClient := itestkitbufconn.NewClient(
		t,
		func(grpcServer *grpcpkg.Server) {
			grpc_health_v1.RegisterHealthServer(grpcServer, &externalHealthServer{
				UnimplementedHealthServer: grpc_health_v1.UnimplementedHealthServer{},
				state:                     externalState,
			})
		},
		grpc_health_v1.NewHealthClient,
		itestkitbufconn.WithBufSize(bufConnSize),
	)

	// Internal server uses externalClient as a dependency, like the real service.
	apiClient := itestkitbufconn.NewClient(
		t,
		func(grpcServer *grpcpkg.Server) {
			grpc_health_v1.RegisterHealthServer(grpcServer, &apiHealthServer{
				UnimplementedHealthServer: grpc_health_v1.UnimplementedHealthServer{},
				externalClient:            externalClient,
			})
		},
		grpc_health_v1.NewHealthClient,
		itestkitbufconn.WithBufSize(bufConnSize),
	)

	return &grpcSyncHarness{
		apiClient:     apiClient,
		externalState: externalState,
	}
}

// PlanExternalCheck is called from prepare steps.
//
// Why expectations are configured in advance:
// the action step should not configure the external mock itself, otherwise tests become unreliable.
func (harness *grpcSyncHarness) PlanExternalCheck(
	plannedCalls []externalPlannedCall,
) {
	harness.externalState.Plan(plannedCalls)
}

// CallAPI is used in the action step.
//
// Important: inside this call, the internal server synchronously calls the external mock server.
func (harness *grpcSyncHarness) CallAPI(
	ctx context.Context,
	serviceChain []string,
) (*grpc_health_v1.HealthCheckResponse, error) {
	if len(serviceChain) == 0 {
		return nil, status.Error(codes.InvalidArgument, "service_chain must contain at least one service")
	}

	joinedServiceChain := strings.Join(serviceChain, serviceChainSeparator)

	return harness.apiClient.Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: joinedServiceChain})
}

// VerifyExternalCheck is used by the verify step.
//
// Why verification is moved to a separate step:
// it allows explicit side-effect checks (outbound call), not only final action response checks.
func (harness *grpcSyncHarness) VerifyExternalCheck(
	expectedServices []string,
	expectedCalls int,
) (*verifyExternalCheckResponse, error) {
	snapshot := harness.externalState.Snapshot()
	if !snapshot.Planned {
		return nil, status.Error(codes.FailedPrecondition, "external expectation is not planned")
	}

	if len(expectedServices) == 0 {
		return nil, status.Error(codes.InvalidArgument, "expected_services must contain at least one value")
	}

	normalizedExpectedServices := make([]string, 0, len(expectedServices))
	for index, expectedService := range expectedServices {
		trimmedService := strings.TrimSpace(expectedService)
		if trimmedService == "" {
			return nil, status.Errorf(codes.InvalidArgument, "expected_services[%d] is empty", index)
		}
		normalizedExpectedServices = append(normalizedExpectedServices, trimmedService)
	}

	if expectedCalls <= 0 {
		expectedCalls = len(normalizedExpectedServices)
	}
	if expectedCalls != len(normalizedExpectedServices) {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"expected_calls must match expected_services length: expected_calls=%d expected_services=%d",
			expectedCalls,
			len(normalizedExpectedServices),
		)
	}

	actualCallsCount := len(snapshot.Calls)
	if actualCallsCount != expectedCalls {
		return nil, status.Errorf(
			codes.FailedPrecondition,
			"external calls count mismatch: want %d got %d",
			expectedCalls,
			actualCallsCount,
		)
	}

	if !slices.Equal(snapshot.Calls, normalizedExpectedServices) {
		return nil, status.Errorf(
			codes.FailedPrecondition,
			"external call sequence mismatch: want %v got %v",
			normalizedExpectedServices,
			snapshot.Calls,
		)
	}

	return &verifyExternalCheckResponse{
		Called:     actualCallsCount > 0,
		CallsCount: actualCallsCount,
		Services:   slices.Clone(snapshot.Calls),
	}, nil
}

// externalHealthServer simulates an external gRPC API.
//
// This server exists to:
// 1) capture actual outbound requests;
// 2) return stub responses from prepare configuration;
// 3) fail immediately on request mismatch.
type externalHealthServer struct {
	grpc_health_v1.UnimplementedHealthServer
	state *externalMockState
}

// Check validates an outbound request and returns a preplanned response.
func (server *externalHealthServer) Check(
	_ context.Context,
	request *grpc_health_v1.HealthCheckRequest,
) (*grpc_health_v1.HealthCheckResponse, error) {
	service := strings.TrimSpace(request.GetService())
	if service == "" {
		return nil, status.Error(codes.InvalidArgument, "service is required")
	}

	stubStatus, err := server.state.ConsumeCall(service)
	if err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}

	return &grpc_health_v1.HealthCheckResponse{Status: stubStatus}, nil
}

// apiHealthServer simulates an internal API handler.
//
// Why it is "thin":
// the main goal is to show outbound-call boundaries, not service business logic.
type apiHealthServer struct {
	grpc_health_v1.UnimplementedHealthServer
	externalClient grpc_health_v1.HealthClient
}

// Check performs one inbound API call that may include multiple outbound calls.
//
// In this example, `request.service` encodes a service sequence using `|` as separator.
// A separate outbound Check to the external API is performed for each service.
func (server *apiHealthServer) Check(
	ctx context.Context,
	request *grpc_health_v1.HealthCheckRequest,
) (*grpc_health_v1.HealthCheckResponse, error) {
	serviceChain, err := parseInboundServiceChain(request.GetService())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	finalStatus := grpc_health_v1.HealthCheckResponse_UNKNOWN
	for _, service := range serviceChain {
		response, externalCheckErr := server.externalClient.Check(ctx, &grpc_health_v1.HealthCheckRequest{
			Service: service,
		})
		if externalCheckErr != nil {
			return nil, fmt.Errorf("external check: %w", externalCheckErr)
		}
		finalStatus = response.GetStatus()
	}

	return &grpc_health_v1.HealthCheckResponse{Status: finalStatus}, nil
}

// externalSnapshot stores an immutable copy of mock state.
//
// Snapshot is needed so verify logic uses consistent race-free state.
type externalSnapshot struct {
	Planned bool
	Plan    []externalPlannedCall
	Calls   []string
}

// externalPlannedCall describes one planned external call entry.
type externalPlannedCall struct {
	ExpectedService string
	StubStatus      grpc_health_v1.HealthCheckResponse_ServingStatus
}

// externalMockState stores mutable external mock state.
//
// State consists of two parts:
// 1) plan (expected request + stub response), defined in prepare step;
// 2) actual call log, filled during runtime outbound calls.
type externalMockState struct {
	mu      sync.Mutex
	planned bool
	plan    []externalPlannedCall
	calls   []string
}

// newExternalMockState creates empty state for the external gRPC mock.
func newExternalMockState() *externalMockState {
	return &externalMockState{
		mu:      sync.Mutex{},
		planned: false,
		plan:    make([]externalPlannedCall, 0),
		calls:   make([]string, 0),
	}
}

// Plan stores expectations and clears the call log.
//
// This prevents case/step leakage through stale call records.
func (state *externalMockState) Plan(plannedCalls []externalPlannedCall) {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.planned = true
	state.plan = slices.Clone(plannedCalls)
	state.calls = state.calls[:0]
}

// ConsumeCall validates the next outbound call and returns matching stub response.
func (state *externalMockState) ConsumeCall(service string) (grpc_health_v1.HealthCheckResponse_ServingStatus, error) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if !state.planned {
		return grpc_health_v1.HealthCheckResponse_UNKNOWN, errors.New("external expectation is not planned")
	}

	callIndex := len(state.calls)
	state.calls = append(state.calls, service)

	if callIndex >= len(state.plan) {
		return grpc_health_v1.HealthCheckResponse_UNKNOWN, fmt.Errorf(
			"unexpected outbound call #%d: got %q, but no more calls are planned",
			callIndex+1,
			service,
		)
	}

	plannedCall := state.plan[callIndex]
	if service != plannedCall.ExpectedService {
		return grpc_health_v1.HealthCheckResponse_UNKNOWN, fmt.Errorf(
			"outbound call #%d mismatch: want %q got %q",
			callIndex+1,
			plannedCall.ExpectedService,
			service,
		)
	}

	return plannedCall.StubStatus, nil
}

// Snapshot returns a thread-safe copy of current mock state.
//
// Verification uses the copy so comparisons stay stable under parallel reads.
func (state *externalMockState) Snapshot() externalSnapshot {
	state.mu.Lock()
	defer state.mu.Unlock()

	return externalSnapshot{
		Planned: state.planned,
		Plan:    slices.Clone(state.plan),
		Calls:   slices.Clone(state.calls),
	}
}

// parseInboundServiceChain parses inbound request service string into an outbound call sequence.
func parseInboundServiceChain(raw string) ([]string, error) {
	trimmedRaw := strings.TrimSpace(raw)
	if trimmedRaw == "" {
		return nil, errors.New("service is required")
	}

	rawServices := strings.Split(trimmedRaw, serviceChainSeparator)
	serviceChain := make([]string, 0, len(rawServices))
	for index, service := range rawServices {
		trimmedService := strings.TrimSpace(service)
		if trimmedService == "" {
			return nil, fmt.Errorf("service chain contains empty item at position %d", index)
		}
		serviceChain = append(serviceChain, trimmedService)
	}

	return serviceChain, nil
}

// grpcSyncHarnessFactory creates a per-case harness and implements suite lifecycle for the example.
type grpcSyncHarnessFactory struct{}

var (
	_ itestkit.HarnessFactory[*grpcSyncHarness]                    = (*grpcSyncHarnessFactory)(nil)
	_ itestkit.SuiteLifecycle[struct{}]                            = (*grpcSyncHarnessFactory)(nil)
	_ itestkit.SuiteCaseHarnessFactory[struct{}, *grpcSyncHarness] = (*grpcSyncHarnessFactory)(nil)
)

// New starts isolated internal API and external mock environment for one case.
func (grpcSyncHarnessFactory) New(t *testing.T) *grpcSyncHarness {
	return newGRPCSyncHarness(t)
}

// SetupSuite prepares suite context; no shared resources are required in this example.
func (grpcSyncHarnessFactory) SetupSuite(_ *testing.T) (struct{}, error) {
	return struct{}{}, nil
}

// TeardownSuite releases suite resources; in this example cleanup happens via t.Cleanup.
func (grpcSyncHarnessFactory) TeardownSuite(_ *testing.T, _ struct{}) error {
	return nil
}

// NewCaseHarness creates a new harness for each case based on suite context.
func (factory grpcSyncHarnessFactory) NewCaseHarness(t *testing.T, _ struct{}) *grpcSyncHarness {
	return factory.New(t)
}
