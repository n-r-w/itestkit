package extapisync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/n-r-w/itestkit"
)

// seedDataHandler binds the prepare step to in-memory "DB" initialization.
type seedDataHandler struct{}

var _ itestkit.Handler[*syncClient] = (*seedDataHandler)(nil)

// DecodeRequest decodes prepare-step payload for loading test orders.
func (seedDataHandler) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &seedDataRequest{Orders: make([]seedOrder, 0)}
	if err := decodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode SeedData request: %w", err)
	}

	return request, nil
}

// DecodeExpectedResponse decodes expected prepare-step response.
func (seedDataHandler) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &seedDataResponse{StoredOrders: 0}
	if len(raw) == 0 {
		return response, nil
	}
	if err := decodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode SeedData response: %w", err)
	}

	return response, nil
}

// Invoke executes the prepare call and validates request type.
func (seedDataHandler) Invoke(ctx context.Context, client *syncClient, request any) (any, error) {
	typedRequest, ok := request.(*seedDataRequest)
	if !ok {
		return nil, fmt.Errorf("SeedData received invalid request type: %T", request)
	}

	return client.SeedData(ctx, typedRequest)
}

// NormalizeResponse returns response unchanged for stable comparison.
func (seedDataHandler) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// planExternalChargeHandler binds the prepare step to external API expectation setup.
type planExternalChargeHandler struct{}

var _ itestkit.Handler[*syncClient] = (*planExternalChargeHandler)(nil)

// DecodeRequest decodes expected input and external API stub response.
func (planExternalChargeHandler) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &planExternalChargeRequest{
		ExpectedRequest: externalChargeRequest{OrderID: "", Amount: 0},
		StubResponse:    externalChargeResponse{Result: "", ApprovalCode: ""},
	}
	if err := decodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode PlanExternalCharge request: %w", err)
	}

	return request, nil
}

// DecodeExpectedResponse decodes expected planning confirmation for the external call.
func (planExternalChargeHandler) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &planExternalChargeResponse{Planned: false}
	if len(raw) == 0 {
		return response, nil
	}
	if err := decodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode PlanExternalCharge response: %w", err)
	}

	return response, nil
}

// Invoke writes external API expectations into internal case state.
func (planExternalChargeHandler) Invoke(ctx context.Context, client *syncClient, request any) (any, error) {
	typedRequest, ok := request.(*planExternalChargeRequest)
	if !ok {
		return nil, fmt.Errorf("PlanExternalCharge received invalid request type: %T", request)
	}

	return client.PlanExternalCharge(ctx, typedRequest)
}

// NormalizeResponse returns planning response unchanged.
func (planExternalChargeHandler) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// createOrderHandler binds the action step to calling "our API".
type createOrderHandler struct{}

var _ itestkit.Handler[*syncClient] = (*createOrderHandler)(nil)

// DecodeRequest decodes action-step input for CreateOrder.
func (createOrderHandler) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &createOrderRequest{OrderID: ""}
	if err := decodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode CreateOrder request: %w", err)
	}

	return request, nil
}

// DecodeExpectedResponse decodes expected "our API" response for assert.response.
func (createOrderHandler) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &createOrderResponse{OrderID: "", ExternalResult: "", ApprovalCode: ""}
	if err := decodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode CreateOrder response: %w", err)
	}

	return response, nil
}

// Invoke calls the action method and validates request type.
func (createOrderHandler) Invoke(ctx context.Context, client *syncClient, request any) (any, error) {
	typedRequest, ok := request.(*createOrderRequest)
	if !ok {
		return nil, fmt.Errorf("CreateOrder received invalid request type: %T", request)
	}

	return client.CreateOrder(ctx, typedRequest)
}

// NormalizeResponse returns CreateOrder response unchanged.
func (createOrderHandler) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// verifyExternalChargeHandler binds the verify step to outbound-call checks.
type verifyExternalChargeHandler struct{}

var _ itestkit.Handler[*syncClient] = (*verifyExternalChargeHandler)(nil)

// DecodeRequest decodes external-call verification parameters.
func (verifyExternalChargeHandler) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &verifyExternalChargeRequest{OrderID: ""}
	if err := decodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode VerifyExternalCharge request: %w", err)
	}

	return request, nil
}

// DecodeExpectedResponse decodes expected verify-step response.
func (verifyExternalChargeHandler) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &verifyExternalChargeResponse{
		Called:      false,
		CallsCount:  0,
		LastRequest: externalChargeRequest{OrderID: "", Amount: 0},
	}
	if len(raw) == 0 {
		return response, nil
	}
	if err := decodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode VerifyExternalCharge response: %w", err)
	}

	return response, nil
}

// Invoke verifies outbound-call fact and payload.
func (verifyExternalChargeHandler) Invoke(ctx context.Context, client *syncClient, request any) (any, error) {
	typedRequest, ok := request.(*verifyExternalChargeRequest)
	if !ok {
		return nil, fmt.Errorf("VerifyExternalCharge received invalid request type: %T", request)
	}

	return client.VerifyExternalCharge(ctx, typedRequest)
}

// NormalizeResponse returns verify response unchanged.
func (verifyExternalChargeHandler) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// decodeStrictJSON enforces strict parsing of step.request and assert.response JSON fields.
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
