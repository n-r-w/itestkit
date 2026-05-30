package extapiasync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/n-r-w/itestkit"
)

// seedDataHandler binds the prepare step to loading initial data.
type seedDataHandler struct{}

var _ itestkit.Handler[*asyncClient] = (*seedDataHandler)(nil)

// DecodeRequest decodes input data for prepare initialization.
func (seedDataHandler) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &seedDataRequest{Orders: make([]seedOrder, 0)}
	if err := decodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode SeedData request: %w", err)
	}

	return request, nil
}

// DecodeExpectedResponse decodes the expected prepare-step response.
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

// Invoke executes loading of initial data.
func (seedDataHandler) Invoke(ctx context.Context, client *asyncClient, request any) (any, error) {
	typedRequest, ok := request.(*seedDataRequest)
	if !ok {
		return nil, fmt.Errorf("SeedData received invalid request type: %T", request)
	}

	return client.SeedData(ctx, typedRequest)
}

// NormalizeResponse returns the response unchanged.
func (seedDataHandler) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// planExternalChargeHandler binds the prepare step to external API expectation setup.
type planExternalChargeHandler struct{}

var _ itestkit.Handler[*asyncClient] = (*planExternalChargeHandler)(nil)

// DecodeRequest decodes expected external request and stub response.
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

// DecodeExpectedResponse decodes expected planning confirmation.
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

// Invoke stores the external call plan in harness state.
func (planExternalChargeHandler) Invoke(ctx context.Context, client *asyncClient, request any) (any, error) {
	typedRequest, ok := request.(*planExternalChargeRequest)
	if !ok {
		return nil, fmt.Errorf("PlanExternalCharge received invalid request type: %T", request)
	}

	return client.PlanExternalCharge(ctx, typedRequest)
}

// NormalizeResponse returns the response unchanged.
func (planExternalChargeHandler) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// submitOrderHandler binds the action step to calling "our API".
type submitOrderHandler struct{}

var _ itestkit.Handler[*asyncClient] = (*submitOrderHandler)(nil)

// DecodeRequest decodes action-step payload.
func (submitOrderHandler) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &submitOrderRequest{OrderID: ""}
	if err := decodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode SubmitOrder request: %w", err)
	}

	return request, nil
}

// DecodeExpectedResponse decodes expected action response.
func (submitOrderHandler) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &submitOrderResponse{OrderID: "", State: ""}
	if len(raw) == 0 {
		return response, nil
	}
	if err := decodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode SubmitOrder response: %w", err)
	}

	return response, nil
}

// Invoke executes the submit endpoint action call.
func (submitOrderHandler) Invoke(ctx context.Context, client *asyncClient, request any) (any, error) {
	typedRequest, ok := request.(*submitOrderRequest)
	if !ok {
		return nil, fmt.Errorf("SubmitOrder received invalid request type: %T", request)
	}

	return client.SubmitOrder(ctx, typedRequest)
}

// NormalizeResponse returns action response unchanged.
func (submitOrderHandler) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// awaitExternalSyncHandler binds the await step to asynchronous processing.
type awaitExternalSyncHandler struct{}

var _ itestkit.Handler[*asyncClient] = (*awaitExternalSyncHandler)(nil)

// DecodeRequest decodes await polling parameters.
func (awaitExternalSyncHandler) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &awaitExternalSyncRequest{OrderID: ""}
	if err := decodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode AwaitExternalSync request: %w", err)
	}

	return request, nil
}

// DecodeExpectedResponse decodes expected await-step state.
func (awaitExternalSyncHandler) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &awaitExternalSyncResponse{OrderID: "", State: "", Attempts: 0}
	if len(raw) == 0 {
		return response, nil
	}
	if err := decodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode AwaitExternalSync response: %w", err)
	}

	return response, nil
}

// Invoke executes await call retried by the runner according to retry policy.
func (awaitExternalSyncHandler) Invoke(ctx context.Context, client *asyncClient, request any) (any, error) {
	typedRequest, ok := request.(*awaitExternalSyncRequest)
	if !ok {
		return nil, fmt.Errorf("AwaitExternalSync received invalid request type: %T", request)
	}

	return client.AwaitExternalSync(ctx, typedRequest)
}

// NormalizeResponse returns await response unchanged.
func (awaitExternalSyncHandler) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// verifyExternalChargeHandler binds the verify step to outbound-call checks.
type verifyExternalChargeHandler struct{}

var _ itestkit.Handler[*asyncClient] = (*verifyExternalChargeHandler)(nil)

// DecodeRequest decodes outbound-call verification parameters.
func (verifyExternalChargeHandler) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &verifyExternalChargeRequest{OrderID: ""}
	if err := decodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode VerifyExternalCharge request: %w", err)
	}

	return request, nil
}

// DecodeExpectedResponse decodes expected verify response.
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

// Invoke checks the observed outbound call.
func (verifyExternalChargeHandler) Invoke(ctx context.Context, client *asyncClient, request any) (any, error) {
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

// getOrderHandler binds the verify step to reading order state from "our API".
type getOrderHandler struct{}

var _ itestkit.Handler[*asyncClient] = (*getOrderHandler)(nil)

// DecodeRequest decodes order-read parameters.
func (getOrderHandler) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &getOrderRequest{OrderID: ""}
	if err := decodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode GetOrder request: %w", err)
	}

	return request, nil
}

// DecodeExpectedResponse decodes expected final response for assert.response.
func (getOrderHandler) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &getOrderResponse{
		OrderID:        "",
		Processed:      false,
		ExternalResult: "",
		ApprovalCode:   "",
	}
	if err := decodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode GetOrder response: %w", err)
	}

	return response, nil
}

// Invoke reads current order state.
func (getOrderHandler) Invoke(ctx context.Context, client *asyncClient, request any) (any, error) {
	typedRequest, ok := request.(*getOrderRequest)
	if !ok {
		return nil, fmt.Errorf("GetOrder received invalid request type: %T", request)
	}

	return client.GetOrder(ctx, typedRequest)
}

// NormalizeResponse returns GetOrder response unchanged.
func (getOrderHandler) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// decodeStrictJSON enforces strict decoding of requests and expected responses.
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
