package extapigrpcadapter

import (
	"context"

	"github.com/n-r-w/itestkit"
	itestkitgrpc "github.com/n-r-w/itestkit/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// normalizedHealthResponse keeps only status for stable JSON assertions.
type normalizedHealthResponse struct {
	Status string `json:"status"`
}

// normalizeHealthResponse converts gRPC response to deterministic shape.
func normalizeHealthResponse(response *grpc_health_v1.HealthCheckResponse) (any, error) {
	return normalizedHealthResponse{Status: response.GetStatus().String()}, nil
}

// newHandlerRegistry returns handlers for adapter-style external calls.
func newHandlerRegistry() itestkit.HandlerRegistry[*adapterHarness] {
	return itestkitgrpc.NewRegistry(map[string]itestkitgrpc.HandlerSpec[*adapterHarness]{
		"CheckExternalThroughAdapter": itestkitgrpc.NewHandlerSpec(
			"CheckExternalThroughAdapter",
			func() *grpc_health_v1.HealthCheckRequest {
				return &grpc_health_v1.HealthCheckRequest{Service: ""}
			},
			func() *grpc_health_v1.HealthCheckResponse {
				return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_UNKNOWN}
			},
			func(
				ctx context.Context,
				harness *adapterHarness,
				request *grpc_health_v1.HealthCheckRequest,
			) (*grpc_health_v1.HealthCheckResponse, error) {
				return harness.adapter.Check(ctx, request)
			},
			normalizeHealthResponse,
		),
	})
}
