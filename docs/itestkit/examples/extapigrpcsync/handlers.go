package extapigrpcsync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/n-r-w/itestkit"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// planExternalCheckStubResponse describes the response that the external gRPC mock should return.
type planExternalCheckStubResponse struct {
	Status string `json:"status"`
}

// planExternalCheckCall describes one expected outbound call.
type planExternalCheckCall struct {
	ExpectedService string                        `json:"expected_service"`
	StubResponse    planExternalCheckStubResponse `json:"stub_response"`
}

// planExternalCheckRequest defines external mock configuration at the prepare step.
//
// calls[] allows planning multiple sequential outbound calls
// with different inputs and different stub responses.
type planExternalCheckRequest struct {
	Calls []planExternalCheckCall `json:"calls"`
}

// planExternalCheckResponse confirms that the external call plan was applied to the harness.
type planExternalCheckResponse struct {
	Planned bool                    `json:"planned"`
	Calls   []planExternalCheckCall `json:"calls"`
}

// callAPIRequest describes action-step input that triggers outbound calls.
type callAPIRequest struct {
	// service supports the legacy format with one external call.
	Service string `json:"service"`
	// service_chain describes a sequence of external calls within one action.
	ServiceChain []string `json:"service_chain"`
}

// normalizedCallAPIResponse stores a compact form of the action response.
//
// This decouples assert from protobuf structure and keeps JSONC comparison stable.
type normalizedCallAPIResponse struct {
	Status string `json:"status"`
}

// verifyExternalCheckRequest defines expectations for dedicated outbound side-effect verification.
type verifyExternalCheckRequest struct {
	ExpectedServices []string `json:"expected_services"`
	ExpectedCalls    int      `json:"expected_calls"`
}

// verifyExternalCheckResponse reflects what actually happened during outbound calls.
type verifyExternalCheckResponse struct {
	Called     bool     `json:"called"`
	CallsCount int      `json:"calls_count"`
	Services   []string `json:"services"`
}

// planExternalCheckHandler binds the prepare step to external gRPC mock configuration.
type planExternalCheckHandler struct{}

var _ itestkit.Handler[*grpcSyncHarness] = (*planExternalCheckHandler)(nil)

// DecodeRequest decodes calls[] from a JSONC step.
//
// Why strict decode matters: fixture errors must fail at load time, not at runtime.
func (planExternalCheckHandler) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &planExternalCheckRequest{
		Calls: make([]planExternalCheckCall, 0),
	}
	if err := decodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode PlanExternalCheck request: %w", err)
	}

	return request, nil
}

// DecodeExpectedResponse decodes expected prepare-step response.
func (planExternalCheckHandler) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &planExternalCheckResponse{
		Planned: false,
		Calls:   make([]planExternalCheckCall, 0),
	}
	if len(raw) == 0 {
		return response, nil
	}
	if err := decodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode PlanExternalCheck response: %w", err)
	}

	return response, nil
}

// Invoke stores external API expectations in the case harness.
//
// This links JSONC prepare-step data with runtime mock logic in externalMockState.
func (planExternalCheckHandler) Invoke(
	_ context.Context,
	harness *grpcSyncHarness,
	request any,
) (any, error) {
	typedRequest, ok := request.(*planExternalCheckRequest)
	if !ok {
		return nil, fmt.Errorf("PlanExternalCheck received invalid request type: %T", request)
	}
	if len(typedRequest.Calls) == 0 {
		return nil, errors.New("calls must contain at least one planned outbound call")
	}

	plannedCalls := make([]externalPlannedCall, 0, len(typedRequest.Calls))
	normalizedCalls := make([]planExternalCheckCall, 0, len(typedRequest.Calls))
	for index, plannedCall := range typedRequest.Calls {
		expectedService := strings.TrimSpace(plannedCall.ExpectedService)
		if expectedService == "" {
			return nil, fmt.Errorf("calls[%d].expected_service is required", index)
		}

		stubStatus, err := parseServingStatus(plannedCall.StubResponse.Status)
		if err != nil {
			return nil, fmt.Errorf("calls[%d].stub_response.status: %w", index, err)
		}

		plannedCalls = append(plannedCalls, externalPlannedCall{
			ExpectedService: expectedService,
			StubStatus:      stubStatus,
		})
		normalizedCalls = append(normalizedCalls, planExternalCheckCall{
			ExpectedService: expectedService,
			StubResponse: planExternalCheckStubResponse{
				Status: stubStatus.String(),
			},
		})
	}

	harness.PlanExternalCheck(plannedCalls)

	return &planExternalCheckResponse{
		Planned: true,
		Calls:   normalizedCalls,
	}, nil
}

// NormalizeResponse returns prepare response unchanged.
func (planExternalCheckHandler) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// callAPIHandler binds the action step to calling the internal gRPC API.
type callAPIHandler struct{}

var _ itestkit.Handler[*grpcSyncHarness] = (*callAPIHandler)(nil)

// DecodeRequest decodes input for an action API call.
//
// The action step should describe only API input, not mock-configuration details.
func (callAPIHandler) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &callAPIRequest{Service: "", ServiceChain: make([]string, 0)}
	if err := decodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode CallAPI request: %w", err)
	}

	return request, nil
}

// DecodeExpectedResponse decodes expected assert.response for the action step.
func (callAPIHandler) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &normalizedCallAPIResponse{Status: ""}
	if err := decodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode CallAPI response: %w", err)
	}

	return response, nil
}

// Invoke executes an internal API gRPC call.
//
// The internal API already has the external mock client injected, so outbound calls happen automatically.
func (callAPIHandler) Invoke(ctx context.Context, harness *grpcSyncHarness, request any) (any, error) {
	typedRequest, ok := request.(*callAPIRequest)
	if !ok {
		return nil, fmt.Errorf("CallAPI received invalid request type: %T", request)
	}

	serviceChain, err := normalizeServiceChain(typedRequest)
	if err != nil {
		return nil, fmt.Errorf("normalize service chain: %w", err)
	}

	return harness.CallAPI(ctx, serviceChain)
}

// NormalizeResponse converts action response to a compact status-only view.
//
// This keeps assert.response transport-agnostic and independent of protobuf details.
func (callAPIHandler) NormalizeResponse(response any) (any, error) {
	switch typedResponse := response.(type) {
	case *grpc_health_v1.HealthCheckResponse:
		return &normalizedCallAPIResponse{Status: typedResponse.GetStatus().String()}, nil
	case *normalizedCallAPIResponse:
		return typedResponse, nil
	case normalizedCallAPIResponse:
		return &typedResponse, nil
	default:
		return nil, fmt.Errorf("unexpected CallAPI response type: %T", response)
	}
}

// verifyExternalCheckHandler binds the verify step to outbound gRPC call checks.
type verifyExternalCheckHandler struct{}

var _ itestkit.Handler[*grpcSyncHarness] = (*verifyExternalCheckHandler)(nil)

// DecodeRequest decodes expected_services and expected_calls for the verify step.
//
// expected_services allows explicit verification of outbound call order.
func (verifyExternalCheckHandler) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &verifyExternalCheckRequest{
		ExpectedServices: make([]string, 0),
		ExpectedCalls:    0,
	}
	if err := decodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode VerifyExternalCheck request: %w", err)
	}

	return request, nil
}

// DecodeExpectedResponse decodes expected verify response.
func (verifyExternalCheckHandler) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &verifyExternalCheckResponse{
		Called:     false,
		CallsCount: 0,
		Services:   make([]string, 0),
	}
	if len(raw) == 0 {
		return response, nil
	}
	if err := decodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode VerifyExternalCheck response: %w", err)
	}

	return response, nil
}

// Invoke verifies that the external API was called with expected parameters.
//
// This separates side-effect checks from action response checks and keeps JSONC scenarios readable.
func (verifyExternalCheckHandler) Invoke(
	_ context.Context,
	harness *grpcSyncHarness,
	request any,
) (any, error) {
	typedRequest, ok := request.(*verifyExternalCheckRequest)
	if !ok {
		return nil, fmt.Errorf("VerifyExternalCheck received invalid request type: %T", request)
	}

	return harness.VerifyExternalCheck(typedRequest.ExpectedServices, typedRequest.ExpectedCalls)
}

// NormalizeResponse returns verify response unchanged.
func (verifyExternalCheckHandler) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// parseServingStatus converts a string status to enum.
//
// Normalization and value whitelisting protect against fixture typos.
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

// normalizeServiceChain builds an external-call sequence for one action.
//
// Two modes are supported:
// 1) service (single call, legacy format);
// 2) service_chain (multiple sequential calls).
func normalizeServiceChain(request *callAPIRequest) ([]string, error) {
	legacyService := strings.TrimSpace(request.Service)
	if legacyService != "" && len(request.ServiceChain) > 0 {
		return nil, errors.New("use either service or service_chain, not both")
	}

	if len(request.ServiceChain) > 0 {
		normalized := make([]string, 0, len(request.ServiceChain))
		for index, service := range request.ServiceChain {
			trimmedService := strings.TrimSpace(service)
			if trimmedService == "" {
				return nil, fmt.Errorf("service_chain[%d] is empty", index)
			}
			normalized = append(normalized, trimmedService)
		}

		return normalized, nil
	}

	if legacyService == "" {
		return nil, errors.New("service or service_chain is required")
	}

	return []string{legacyService}, nil
}

// decodeStrictJSON enforces strict decoding of step.request and assert.response fields.
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
