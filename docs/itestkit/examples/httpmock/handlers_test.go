package httpmockexample

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/n-r-w/itestkit"
)

type createOrderHandler struct{}

// DecodeRequest decodes a create-order request from JSONC.
func (createOrderHandler) DecodeRequest(raw json.RawMessage) (any, error) {
	var request createOrderRequest
	if err := itestkit.DecodeStrictJSON(raw, &request); err != nil {
		return nil, err
	}
	return request, nil
}

// DecodeExpectedResponse decodes an expected create-order response from JSONC.
func (createOrderHandler) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	var response createOrderResponse
	if err := itestkit.DecodeStrictJSON(raw, &response); err != nil {
		return nil, err
	}
	return response, nil
}

// Invoke creates an order through the example client.
func (createOrderHandler) Invoke(ctx context.Context, testHarness *harness, request any) (any, error) {
	typedRequest, ok := request.(createOrderRequest)
	if !ok {
		return nil, fmt.Errorf("CreateOrder received invalid request type: %T", request)
	}
	return testHarness.client.CreateOrder(ctx, typedRequest)
}

// NormalizeResponse returns the create-order response unchanged.
func (createOrderHandler) NormalizeResponse(response any) (any, error) {
	return response, nil
}
