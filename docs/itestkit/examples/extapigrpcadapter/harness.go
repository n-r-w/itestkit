package extapigrpcadapter

import (
	"context"
	"fmt"
	"testing"

	"github.com/n-r-w/itestkit"
	itestkitbufconn "github.com/n-r-w/itestkit/grpc/bufconn"
	"github.com/stretchr/testify/require"
	grpcpkg "google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
)

const adapterBufConnSize = 1024 * 1024

// adapterHarness stores dependencies for one case.
type adapterHarness struct {
	adapter *externalHealthAdapter
}

// externalHealthAdapter models an adapter-style dependency with target + dial options constructor.
type externalHealthAdapter struct {
	conn   *grpcpkg.ClientConn
	client grpc_health_v1.HealthClient
}

// newExternalHealthAdapter creates an adapter from transport-level target and dial options.
func newExternalHealthAdapter(target string, dialOptions ...grpcpkg.DialOption) (*externalHealthAdapter, error) {
	conn, err := grpcpkg.NewClient(target, dialOptions...)
	if err != nil {
		return nil, fmt.Errorf("create adapter grpc client: %w", err)
	}

	return &externalHealthAdapter{
		conn:   conn,
		client: grpc_health_v1.NewHealthClient(conn),
	}, nil
}

// Check delegates the request to the underlying gRPC client.
func (adapter *externalHealthAdapter) Check(
	ctx context.Context,
	request *grpc_health_v1.HealthCheckRequest,
) (*grpc_health_v1.HealthCheckResponse, error) {
	return adapter.client.Check(ctx, request)
}

// Close releases adapter connection resources.
func (adapter *externalHealthAdapter) Close() error {
	if adapter == nil || adapter.conn == nil {
		return nil
	}

	return adapter.conn.Close()
}

// mockExternalHealthServer simulates an external gRPC dependency.
type mockExternalHealthServer struct {
	grpc_health_v1.UnimplementedHealthServer
}

// Check returns a fixed healthy status for deterministic assertions.
func (mockExternalHealthServer) Check(
	_ context.Context,
	_ *grpc_health_v1.HealthCheckRequest,
) (*grpc_health_v1.HealthCheckResponse, error) {
	return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}, nil
}

// newAdapterHarness creates one isolated harness instance.
func newAdapterHarness(t *testing.T) *adapterHarness {
	server := itestkitbufconn.NewServer(
		t,
		func(grpcServer *grpcpkg.Server) {
			grpc_health_v1.RegisterHealthServer(
				grpcServer,
				mockExternalHealthServer{UnimplementedHealthServer: grpc_health_v1.UnimplementedHealthServer{}},
			)
		},
		itestkitbufconn.WithBufSize(adapterBufConnSize),
		itestkitbufconn.WithTarget("passthrough:///bufnet-external-adapter"),
	)

	adapter, err := newExternalHealthAdapter(server.Target(), server.DialOptions()...)
	require.NoError(t, err)
	t.Cleanup(func() {
		if closeErr := adapter.Close(); closeErr != nil {
			t.Errorf("close adapter: %v", closeErr)
		}
	})

	return &adapterHarness{adapter: adapter}
}

// harnessFactory creates case-level dependencies for the adapter-style example.
type harnessFactory struct{}

var (
	_ itestkit.HarnessFactory[*adapterHarness]                    = (*harnessFactory)(nil)
	_ itestkit.SuiteLifecycle[struct{}]                           = (*harnessFactory)(nil)
	_ itestkit.SuiteCaseHarnessFactory[struct{}, *adapterHarness] = (*harnessFactory)(nil)
)

// New creates a fresh harness per case.
func (harnessFactory) New(t *testing.T) *adapterHarness {
	return newAdapterHarness(t)
}

// SetupSuite prepares suite context (unused in this example).
func (harnessFactory) SetupSuite(_ *testing.T) (struct{}, error) {
	return struct{}{}, nil
}

// TeardownSuite releases suite resources (none in this example).
func (harnessFactory) TeardownSuite(_ *testing.T, _ struct{}) error {
	return nil
}

// NewCaseHarness creates a case harness from suite context.
func (factory harnessFactory) NewCaseHarness(t *testing.T, _ struct{}) *adapterHarness {
	return factory.New(t)
}
