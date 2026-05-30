package itests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/n-r-w/itestkit"
	"github.com/n-r-w/itestkit/docs/itestkit/examples/real/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

// seedOrder describes a record for in-memory DB seeding.
type seedOrder struct {
	OrderID string `json:"order_id"`
	Amount  int64  `json:"amount"`
}

// seedOrdersRequest describes the SeedOrders prepare-step request.
type seedOrdersRequest struct {
	Orders []seedOrder `json:"orders"`
}

// seedOrdersResponse describes the SeedOrders prepare-step response.
type seedOrdersResponse struct {
	Seeded int         `json:"seeded"`
	Orders []seedOrder `json:"orders"`
}

// planExternalStubResponse describes a mock response from external API.
type planExternalStubResponse struct {
	Status string `json:"status"`
}

// planExternalCall describes one expected outbound call.
type planExternalCall struct {
	ExpectedService string                   `json:"expected_service"`
	StubResponse    planExternalStubResponse `json:"stub_response"`
}

// planExternalCallsRequest describes the PlanExternalCalls prepare-step request.
type planExternalCallsRequest struct {
	Calls []planExternalCall `json:"calls"`
}

// planExternalCallsResponse describes the PlanExternalCalls prepare-step response.
type planExternalCallsResponse struct {
	Planned bool               `json:"planned"`
	Calls   []planExternalCall `json:"calls"`
}

// callOrderAPIRequest describes an action request to service API.
type callOrderAPIRequest struct {
	OrderID string `json:"order_id"`
}

// normalizedCallOrderAPIResponse stores a compact action-response view.
type normalizedCallOrderAPIResponse struct {
	Status string `json:"status"`
}

// verifyExternalCallsRequest describes external-call verification step.
type verifyExternalCallsRequest struct {
	ExpectedServices []string `json:"expected_services"`
	ExpectedCalls    int      `json:"expected_calls"`
}

// verifyExternalCallsResponse describes external-call verification result.
type verifyExternalCallsResponse struct {
	Called     bool     `json:"called"`
	CallsCount int      `json:"calls_count"`
	Services   []string `json:"services"`
}

// verifyOrderStateRequest describes order-state verification step in DB.
type verifyOrderStateRequest struct {
	OrderID                  string   `json:"order_id"`
	ExpectedStatus           string   `json:"expected_status"`
	ExpectedExternalServices []string `json:"expected_external_services"`
}

// verifyOrderStateResponse describes actual order state in DB.
type verifyOrderStateResponse struct {
	OrderID          string   `json:"order_id"`
	Amount           int64    `json:"amount"`
	Status           string   `json:"status"`
	ExternalServices []string `json:"external_services"`
}

// seedOrdersHandler prepares the in-memory DB.
type seedOrdersHandler struct{}

var _ itestkit.Handler[*caseHarness] = (*seedOrdersHandler)(nil)

// DecodeRequest decodes SeedOrders request JSON.
func (seedOrdersHandler) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &seedOrdersRequest{Orders: make([]seedOrder, 0)}
	if err := decodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode SeedOrders request: %w", err)
	}

	return request, nil
}

// DecodeExpectedResponse decodes expected SeedOrders response.
func (seedOrdersHandler) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &seedOrdersResponse{Seeded: 0, Orders: make([]seedOrder, 0)}
	if len(raw) == 0 {
		return response, nil
	}
	if err := decodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode SeedOrders response: %w", err)
	}

	return response, nil
}

// Invoke seeds test orders into the DB.
func (seedOrdersHandler) Invoke(
	ctx context.Context,
	harness *caseHarness,
	request any,
) (any, error) {
	typedRequest, ok := request.(*seedOrdersRequest)
	if !ok {
		return nil, fmt.Errorf("SeedOrders received invalid request type: %T", request)
	}
	if len(typedRequest.Orders) == 0 {
		return nil, status.Error(codes.InvalidArgument, "orders must contain at least one item")
	}

	normalizedOrders := make([]seedOrder, 0, len(typedRequest.Orders))
	for index, order := range typedRequest.Orders {
		orderID := strings.TrimSpace(order.OrderID)
		if orderID == "" {
			return nil, status.Errorf(codes.InvalidArgument, "orders[%d].order_id is required", index)
		}

		saveErr := harness.repository.Save(
			ctx,
			service.Order{
				ID:               orderID,
				Amount:           order.Amount,
				Status:           grpc_health_v1.HealthCheckResponse_UNKNOWN,
				ExternalServices: make([]string, 0),
			},
		)
		if saveErr != nil {
			return nil, status.Errorf(codes.Internal, "save order %q: %v", orderID, saveErr)
		}

		normalizedOrders = append(normalizedOrders, seedOrder{
			OrderID: orderID,
			Amount:  order.Amount,
		})
	}

	return &seedOrdersResponse{
		Seeded: len(normalizedOrders),
		Orders: normalizedOrders,
	}, nil
}

// NormalizeResponse returns SeedOrders response unchanged.
func (seedOrdersHandler) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// planExternalCallsHandler configures external gRPC mock.
type planExternalCallsHandler struct{}

var _ itestkit.Handler[*caseHarness] = (*planExternalCallsHandler)(nil)

// DecodeRequest decodes PlanExternalCalls request JSON.
func (planExternalCallsHandler) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &planExternalCallsRequest{Calls: make([]planExternalCall, 0)}
	if err := decodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode PlanExternalCalls request: %w", err)
	}

	return request, nil
}

// DecodeExpectedResponse decodes expected PlanExternalCalls response.
func (planExternalCallsHandler) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &planExternalCallsResponse{Planned: false, Calls: make([]planExternalCall, 0)}
	if len(raw) == 0 {
		return response, nil
	}
	if err := decodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode PlanExternalCalls response: %w", err)
	}

	return response, nil
}

// Invoke stores the external call plan.
func (planExternalCallsHandler) Invoke(
	_ context.Context,
	harness *caseHarness,
	request any,
) (any, error) {
	typedRequest, ok := request.(*planExternalCallsRequest)
	if !ok {
		return nil, fmt.Errorf("PlanExternalCalls received invalid request type: %T", request)
	}
	if len(typedRequest.Calls) == 0 {
		return nil, status.Error(codes.InvalidArgument, "calls must contain at least one item")
	}

	plannedCalls := make([]externalPlannedCall, 0, len(typedRequest.Calls))
	normalizedCalls := make([]planExternalCall, 0, len(typedRequest.Calls))
	for index, call := range typedRequest.Calls {
		expectedService := strings.TrimSpace(call.ExpectedService)
		if expectedService == "" {
			return nil, status.Errorf(codes.InvalidArgument, "calls[%d].expected_service is required", index)
		}

		stubStatus, parseErr := parseServingStatus(call.StubResponse.Status)
		if parseErr != nil {
			return nil, status.Errorf(codes.InvalidArgument, "calls[%d].stub_response.status: %v", index, parseErr)
		}

		plannedCalls = append(plannedCalls, externalPlannedCall{
			ExpectedService: expectedService,
			StubStatus:      stubStatus,
		})
		normalizedCalls = append(normalizedCalls, planExternalCall{
			ExpectedService: expectedService,
			StubResponse: planExternalStubResponse{
				Status: stubStatus.String(),
			},
		})
	}

	harness.externalState.Plan(plannedCalls)

	return &planExternalCallsResponse{
		Planned: true,
		Calls:   normalizedCalls,
	}, nil
}

// NormalizeResponse returns PlanExternalCalls response unchanged.
func (planExternalCallsHandler) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// callOrderAPIHandler calls the service gRPC API.
type callOrderAPIHandler struct{}

var _ itestkit.Handler[*caseHarness] = (*callOrderAPIHandler)(nil)

// DecodeRequest decodes CallOrderAPI request JSON.
func (callOrderAPIHandler) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &callOrderAPIRequest{OrderID: ""}
	if err := decodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode CallOrderAPI request: %w", err)
	}

	return request, nil
}

// DecodeExpectedResponse decodes expected CallOrderAPI response.
func (callOrderAPIHandler) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &normalizedCallOrderAPIResponse{Status: ""}
	if err := decodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode CallOrderAPI response: %w", err)
	}

	return response, nil
}

// Invoke executes action API call.
func (callOrderAPIHandler) Invoke(ctx context.Context, harness *caseHarness, request any) (any, error) {
	typedRequest, ok := request.(*callOrderAPIRequest)
	if !ok {
		return nil, fmt.Errorf("CallOrderAPI received invalid request type: %T", request)
	}

	orderID := strings.TrimSpace(typedRequest.OrderID)
	if orderID == "" {
		return nil, status.Error(codes.InvalidArgument, "order_id is required")
	}

	return harness.apiClient.Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: orderID})
}

// NormalizeResponse normalizes protobuf CallOrderAPI response.
func (callOrderAPIHandler) NormalizeResponse(response any) (any, error) {
	switch typedResponse := response.(type) {
	case *grpc_health_v1.HealthCheckResponse:
		return &normalizedCallOrderAPIResponse{Status: typedResponse.GetStatus().String()}, nil
	case *normalizedCallOrderAPIResponse:
		return typedResponse, nil
	case normalizedCallOrderAPIResponse:
		return &typedResponse, nil
	default:
		return nil, fmt.Errorf("unexpected CallOrderAPI response type: %T", response)
	}
}

// verifyExternalCallsHandler checks outbound call fact, order, and count.
type verifyExternalCallsHandler struct{}

var _ itestkit.Handler[*caseHarness] = (*verifyExternalCallsHandler)(nil)

// DecodeRequest decodes VerifyExternalCalls request JSON.
func (verifyExternalCallsHandler) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &verifyExternalCallsRequest{ExpectedServices: make([]string, 0), ExpectedCalls: 0}
	if err := decodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode VerifyExternalCalls request: %w", err)
	}

	return request, nil
}

// DecodeExpectedResponse decodes expected VerifyExternalCalls response.
func (verifyExternalCallsHandler) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &verifyExternalCallsResponse{
		Called:     false,
		CallsCount: 0,
		Services:   make([]string, 0),
	}
	if len(raw) == 0 {
		return response, nil
	}
	if err := decodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode VerifyExternalCalls response: %w", err)
	}

	return response, nil
}

// Invoke verifies external API side effects.
func (verifyExternalCallsHandler) Invoke(
	_ context.Context,
	harness *caseHarness,
	request any,
) (any, error) {
	typedRequest, ok := request.(*verifyExternalCallsRequest)
	if !ok {
		return nil, fmt.Errorf("VerifyExternalCalls received invalid request type: %T", request)
	}

	snapshot := harness.externalState.Snapshot()
	if !snapshot.Planned {
		return nil, status.Error(codes.FailedPrecondition, "external calls are not planned")
	}
	if len(typedRequest.ExpectedServices) == 0 {
		return nil, status.Error(codes.InvalidArgument, "expected_services must contain at least one value")
	}

	normalizedExpectedServices := make([]string, 0, len(typedRequest.ExpectedServices))
	for index, expectedService := range typedRequest.ExpectedServices {
		trimmedService := strings.TrimSpace(expectedService)
		if trimmedService == "" {
			return nil, status.Errorf(codes.InvalidArgument, "expected_services[%d] is empty", index)
		}
		normalizedExpectedServices = append(normalizedExpectedServices, trimmedService)
	}

	expectedCalls := typedRequest.ExpectedCalls
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

	return &verifyExternalCallsResponse{
		Called:     actualCallsCount > 0,
		CallsCount: actualCallsCount,
		Services:   slices.Clone(snapshot.Calls),
	}, nil
}

// NormalizeResponse returns VerifyExternalCalls response unchanged.
func (verifyExternalCallsHandler) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// verifyOrderStateHandler checks order state in in-memory DB after action step.
type verifyOrderStateHandler struct{}

var _ itestkit.Handler[*caseHarness] = (*verifyOrderStateHandler)(nil)

// DecodeRequest decodes VerifyOrderState request JSON.
func (verifyOrderStateHandler) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &verifyOrderStateRequest{
		OrderID:                  "",
		ExpectedStatus:           "",
		ExpectedExternalServices: make([]string, 0),
	}
	if err := decodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode VerifyOrderState request: %w", err)
	}

	return request, nil
}

// DecodeExpectedResponse decodes expected VerifyOrderState response.
func (verifyOrderStateHandler) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &verifyOrderStateResponse{
		OrderID:          "",
		Amount:           0,
		Status:           "",
		ExternalServices: make([]string, 0),
	}
	if len(raw) == 0 {
		return response, nil
	}
	if err := decodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode VerifyOrderState response: %w", err)
	}

	return response, nil
}

// Invoke verifies order state in the DB.
func (verifyOrderStateHandler) Invoke(
	ctx context.Context,
	harness *caseHarness,
	request any,
) (any, error) {
	typedRequest, ok := request.(*verifyOrderStateRequest)
	if !ok {
		return nil, fmt.Errorf("VerifyOrderState received invalid request type: %T", request)
	}

	orderID := strings.TrimSpace(typedRequest.OrderID)
	if orderID == "" {
		return nil, status.Error(codes.InvalidArgument, "order_id is required")
	}

	order, err := harness.repository.Get(ctx, orderID)
	if err != nil {
		if service.IsOrderNotFoundError(err) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "get order: %v", err)
	}

	expectedStatus, parseErr := parseServingStatus(typedRequest.ExpectedStatus)
	if parseErr != nil {
		return nil, status.Errorf(codes.InvalidArgument, "expected_status: %v", parseErr)
	}
	if order.Status != expectedStatus {
		return nil, status.Errorf(
			codes.FailedPrecondition,
			"order status mismatch: want %s got %s",
			expectedStatus.String(),
			order.Status.String(),
		)
	}

	if len(typedRequest.ExpectedExternalServices) == 0 {
		return nil, status.Error(codes.InvalidArgument, "expected_external_services must contain at least one value")
	}

	normalizedExpectedExternalServices := make([]string, 0, len(typedRequest.ExpectedExternalServices))
	for index, expectedService := range typedRequest.ExpectedExternalServices {
		trimmedService := strings.TrimSpace(expectedService)
		if trimmedService == "" {
			return nil, status.Errorf(codes.InvalidArgument, "expected_external_services[%d] is empty", index)
		}
		normalizedExpectedExternalServices = append(normalizedExpectedExternalServices, trimmedService)
	}
	if !slices.Equal(order.ExternalServices, normalizedExpectedExternalServices) {
		return nil, status.Errorf(
			codes.FailedPrecondition,
			"order external services mismatch: want %v got %v",
			normalizedExpectedExternalServices,
			order.ExternalServices,
		)
	}

	return &verifyOrderStateResponse{
		OrderID:          order.ID,
		Amount:           order.Amount,
		Status:           order.Status.String(),
		ExternalServices: slices.Clone(order.ExternalServices),
	}, nil
}

// NormalizeResponse returns VerifyOrderState response unchanged.
func (verifyOrderStateHandler) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// parseServingStatus converts a string into gRPC health status enum.
func parseServingStatus(raw string) (grpc_health_v1.HealthCheckResponse_ServingStatus, error) {
	normalized := strings.TrimSpace(strings.ToUpper(raw))
	switch normalized {
	case grpc_health_v1.HealthCheckResponse_UNKNOWN.String():
		return grpc_health_v1.HealthCheckResponse_UNKNOWN, nil
	case grpc_health_v1.HealthCheckResponse_SERVING.String():
		return grpc_health_v1.HealthCheckResponse_SERVING, nil
	case grpc_health_v1.HealthCheckResponse_NOT_SERVING.String():
		return grpc_health_v1.HealthCheckResponse_NOT_SERVING, nil
	case grpc_health_v1.HealthCheckResponse_SERVICE_UNKNOWN.String():
		return grpc_health_v1.HealthCheckResponse_SERVICE_UNKNOWN, nil
	default:
		return grpc_health_v1.HealthCheckResponse_UNKNOWN, fmt.Errorf(
			"unsupported status %q; allowed values: UNKNOWN, SERVING, NOT_SERVING, SERVICE_UNKNOWN",
			raw,
		)
	}
}

// decodeStrictJSON performs strict JSON decode:
// 1) disallows unknown fields;
// 2) disallows trailing data.
func decodeStrictJSON(raw json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode json: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("decode json: trailing data")
	}

	return nil
}
