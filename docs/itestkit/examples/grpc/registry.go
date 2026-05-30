// Package grpc shows a minimal itestkit usage example with gRPC.
package grpc

import (
	"context"

	"github.com/n-r-w/itestkit"
	itestkitgrpc "github.com/n-r-w/itestkit/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// normalizedHealthResponse keeps only status for stable comparison.
type normalizedHealthResponse struct {
	Status string `json:"status"`
}

// normalizeHealthResponse converts a gRPC response to a compact form.
func normalizeHealthResponse(response *grpc_health_v1.HealthCheckResponse) (any, error) {
	return normalizedHealthResponse{Status: response.GetStatus().String()}, nil
}

// newHandlerRegistry builds gRPC handlers for the example.
func newHandlerRegistry() itestkit.HandlerRegistry[grpc_health_v1.HealthClient] {
	return itestkitgrpc.NewRegistry(map[string]itestkitgrpc.HandlerSpec[grpc_health_v1.HealthClient]{
		"Check": itestkitgrpc.NewHandlerSpec(
			"Check",
			func() *grpc_health_v1.HealthCheckRequest {
				return &grpc_health_v1.HealthCheckRequest{Service: ""}
			},
			func() *grpc_health_v1.HealthCheckResponse {
				return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_UNKNOWN}
			},
			func(
				ctx context.Context,
				client grpc_health_v1.HealthClient,
				request *grpc_health_v1.HealthCheckRequest,
			) (*grpc_health_v1.HealthCheckResponse, error) {
				return client.Check(ctx, request)
			},
			normalizeHealthResponse,
		),
	})
}
