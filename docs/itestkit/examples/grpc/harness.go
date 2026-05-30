package grpc

import (
	"context"
	"testing"

	"github.com/n-r-w/itestkit"
	itestkitbufconn "github.com/n-r-w/itestkit/grpc/bufconn"
	grpcpkg "google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
)

const bufConnSize = 1024 * 1024

// healthServer responds to health-check requests using a fixed status.
type healthServer struct {
	grpc_health_v1.UnimplementedHealthServer
	status grpc_health_v1.HealthCheckResponse_ServingStatus
}

// Check returns service status without additional dependencies.
func (server *healthServer) Check(
	_ context.Context,
	_ *grpc_health_v1.HealthCheckRequest,
) (*grpc_health_v1.HealthCheckResponse, error) {
	return &grpc_health_v1.HealthCheckResponse{Status: server.status}, nil
}

// grpcHarnessFactory creates a gRPC client over bufconn.
type grpcHarnessFactory struct{}

var (
	_ itestkit.HarnessFactory[grpc_health_v1.HealthClient]                    = (*grpcHarnessFactory)(nil)
	_ itestkit.SuiteLifecycle[struct{}]                                       = (*grpcHarnessFactory)(nil)
	_ itestkit.SuiteCaseHarnessFactory[struct{}, grpc_health_v1.HealthClient] = (*grpcHarnessFactory)(nil)
)

// New starts a minimal gRPC server and returns a case client.
func (grpcHarnessFactory) New(t *testing.T) grpc_health_v1.HealthClient {
	server := &healthServer{
		UnimplementedHealthServer: grpc_health_v1.UnimplementedHealthServer{},
		status:                    grpc_health_v1.HealthCheckResponse_SERVING,
	}

	client := itestkitbufconn.NewClient(
		t,
		func(grpcServer *grpcpkg.Server) {
			grpc_health_v1.RegisterHealthServer(grpcServer, server)
		},
		grpc_health_v1.NewHealthClient,
		itestkitbufconn.WithBufSize(bufConnSize),
	)

	return client
}

// SetupSuite prepares suite context for the gRPC example.
func (grpcHarnessFactory) SetupSuite(_ *testing.T) (struct{}, error) {
	return struct{}{}, nil
}

// TeardownSuite releases suite resources (in this example resources are released per case harness).
func (grpcHarnessFactory) TeardownSuite(_ *testing.T, _ struct{}) error {
	return nil
}

// NewCaseHarness creates a harness for a specific case based on suite context.
func (factory grpcHarnessFactory) NewCaseHarness(t *testing.T, _ struct{}) grpc_health_v1.HealthClient {
	return factory.New(t)
}
