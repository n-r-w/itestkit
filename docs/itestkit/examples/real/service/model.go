// Package service contains an example of a production-like gRPC service for integration tests.
package service

import (
	"fmt"

	"google.golang.org/grpc/health/grpc_health_v1"
)

const (
	manualReviewAmountThreshold  int64 = 5000
	externalServicePrefixRisk          = "risk"
	externalServicePrefixReserve       = "reserve"
	externalServicePrefixManual        = "manual"
	defaultExternalChainCapacity       = 3
)

// Order stores order state in an in-memory repository.
type Order struct {
	ID               string
	Amount           int64
	Status           grpc_health_v1.HealthCheckResponse_ServingStatus
	ExternalServices []string
}

// BuildExternalServiceChain builds an outbound-call sequence from order data.
//
// For standard orders, two calls are executed:
// 1) risk:<order-id>
// 2) reserve:<order-id>
//
// For large orders, a third call is added:
// 3) manual:<order-id>.
func BuildExternalServiceChain(orderID string, amount int64) []string {
	chain := make([]string, 0, defaultExternalChainCapacity)
	chain = append(
		chain,
		fmt.Sprintf("%s:%s", externalServicePrefixRisk, orderID),
		fmt.Sprintf("%s:%s", externalServicePrefixReserve, orderID),
	)
	if amount >= manualReviewAmountThreshold {
		chain = append(chain, fmt.Sprintf("%s:%s", externalServicePrefixManual, orderID))
	}

	return chain
}
