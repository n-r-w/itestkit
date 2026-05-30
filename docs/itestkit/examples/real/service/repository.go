package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"google.golang.org/grpc/health/grpc_health_v1"
)

// InMemoryOrderRepository stores orders in memory and is used in integration tests.
type InMemoryOrderRepository struct {
	mu     sync.Mutex
	orders map[string]Order
}

// NewInMemoryOrderRepository creates an empty in-memory repository.
func NewInMemoryOrderRepository() *InMemoryOrderRepository {
	return &InMemoryOrderRepository{
		mu:     sync.Mutex{},
		orders: make(map[string]Order),
	}
}

// Save stores an order in the repository.
func (repository *InMemoryOrderRepository) Save(_ context.Context, order Order) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	repository.orders[order.ID] = cloneOrder(order)

	return nil
}

// Get returns an order by ID.
func (repository *InMemoryOrderRepository) Get(_ context.Context, orderID string) (Order, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	order, exists := repository.orders[orderID]
	if !exists {
		return Order{}, orderNotFoundError{OrderID: orderID}
	}

	return cloneOrder(order), nil
}

// UpdateProcessingResult updates order status and outbound-call log.
func (repository *InMemoryOrderRepository) UpdateProcessingResult(
	_ context.Context,
	orderID string,
	status grpc_health_v1.HealthCheckResponse_ServingStatus,
	externalServices []string,
) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	order, exists := repository.orders[orderID]
	if !exists {
		return orderNotFoundError{OrderID: orderID}
	}

	order.Status = status
	order.ExternalServices = slices.Clone(externalServices)
	repository.orders[orderID] = cloneOrder(order)

	return nil
}

// IsOrderNotFoundError returns true when error indicates a missing order.
func IsOrderNotFoundError(err error) bool {
	var target orderNotFoundError

	return errors.As(err, &target)
}

// orderNotFoundError describes a domain error for a missing order.
type orderNotFoundError struct {
	OrderID string
}

// Error returns an error message for a missing order.
func (err orderNotFoundError) Error() string {
	return fmt.Sprintf("order %q is not found", err.OrderID)
}

// cloneOrder makes a safe copy so callers do not share mutable slices.
func cloneOrder(order Order) Order {
	return Order{
		ID:               order.ID,
		Amount:           order.Amount,
		Status:           order.Status,
		ExternalServices: slices.Clone(order.ExternalServices),
	}
}
