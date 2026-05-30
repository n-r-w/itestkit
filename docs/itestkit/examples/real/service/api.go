package service

import (
	"context"
	"strings"

	grpcpkg "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

// orderRepository defines repository contract within API-layer boundaries.
type orderRepository interface {
	Get(ctx context.Context, orderID string) (Order, error)
	UpdateProcessingResult(
		ctx context.Context,
		orderID string,
		status grpc_health_v1.HealthCheckResponse_ServingStatus,
		externalServices []string,
	) error
}

// APIServer simulates a real service gRPC API.
//
// Input:
// - request.service = order_id
//
// Behavior:
// 1) reads an order from repository;
// 2) builds external call chain;
// 3) makes outbound gRPC calls to external API;
// 4) stores final result in repository;
// 5) returns final status.
type APIServer struct {
	grpc_health_v1.UnimplementedHealthServer
	repository     orderRepository
	externalClient grpc_health_v1.HealthClient
}

// NewAPIServer creates the service API server.
func NewAPIServer(
	repository orderRepository,
	externalClient grpc_health_v1.HealthClient,
) *APIServer {
	return &APIServer{
		UnimplementedHealthServer: grpc_health_v1.UnimplementedHealthServer{},
		repository:                repository,
		externalClient:            externalClient,
	}
}

// Check processes an inbound API request and executes external calls.
func (server *APIServer) Check(
	ctx context.Context,
	request *grpc_health_v1.HealthCheckRequest,
) (*grpc_health_v1.HealthCheckResponse, error) {
	orderID := strings.TrimSpace(request.GetService())
	if orderID == "" {
		return nil, status.Error(codes.InvalidArgument, "service (order_id) is required")
	}

	order, err := server.repository.Get(ctx, orderID)
	if err != nil {
		if IsOrderNotFoundError(err) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "get order: %v", err)
	}

	externalServiceChain := BuildExternalServiceChain(order.ID, order.Amount)
	if len(externalServiceChain) == 0 {
		return nil, status.Error(codes.Internal, "external service chain is empty")
	}

	finalStatus := grpc_health_v1.HealthCheckResponse_UNKNOWN
	for _, externalService := range externalServiceChain {
		externalResponse, externalErr := server.externalClient.Check(
			ctx,
			&grpc_health_v1.HealthCheckRequest{Service: externalService},
			grpcpkg.WaitForReady(false),
		)
		if externalErr != nil {
			return nil, status.Errorf(codes.Unavailable, "external call %q failed: %v", externalService, externalErr)
		}
		finalStatus = externalResponse.GetStatus()
	}

	if updateErr := server.repository.UpdateProcessingResult(
		ctx, order.ID, finalStatus, externalServiceChain); updateErr != nil {
		return nil, status.Errorf(codes.Internal, "update order: %v", updateErr)
	}

	return &grpc_health_v1.HealthCheckResponse{Status: finalStatus}, nil
}
