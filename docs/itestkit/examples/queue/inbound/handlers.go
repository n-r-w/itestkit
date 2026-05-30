package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/n-r-w/itestkit"
)

// initEnvironmentHandler binds the prepare step to in-memory environment initialization.
type initEnvironmentHandler struct{}

var _ itestkit.Handler[*queueClient] = (*initEnvironmentHandler)(nil)

// DecodeRequest decodes the prepare-step request.
func (initEnvironmentHandler) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &initEnvironmentRequest{}
	if len(raw) == 0 {
		return request, nil
	}
	if err := decodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode InitEnvironment request: %w", err)
	}

	return request, nil
}

// DecodeExpectedResponse decodes the expected prepare-step response.
func (initEnvironmentHandler) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &initEnvironmentResponse{QueueDepth: 0}
	if len(raw) == 0 {
		return response, nil
	}
	if err := decodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode InitEnvironment response: %w", err)
	}

	return response, nil
}

// Invoke executes the prepare call and validates request type.
func (initEnvironmentHandler) Invoke(ctx context.Context, client *queueClient, request any) (any, error) {
	typedRequest, ok := request.(*initEnvironmentRequest)
	if !ok {
		return nil, fmt.Errorf("InitEnvironment received invalid request type: %T", request)
	}

	return client.InitEnvironment(ctx, typedRequest)
}

// NormalizeResponse returns the response unchanged because it is already stable.
func (initEnvironmentHandler) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// publishOrderHandler binds the publish step to writing a message into the in-memory queue.
type publishOrderHandler struct{}

var _ itestkit.Handler[*queueClient] = (*publishOrderHandler)(nil)

// DecodeRequest decodes the publish request.
func (publishOrderHandler) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &publishOrderRequest{OrderID: "", Amount: 0}
	if err := decodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode PublishOrder request: %w", err)
	}

	return request, nil
}

// DecodeExpectedResponse decodes the expected publish-step result.
func (publishOrderHandler) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &publishOrderResponse{Topic: "", Offset: 0}
	if len(raw) == 0 {
		return response, nil
	}
	if err := decodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode PublishOrder response: %w", err)
	}

	return response, nil
}

// Invoke publishes the message to the queue.
func (publishOrderHandler) Invoke(ctx context.Context, client *queueClient, request any) (any, error) {
	typedRequest, ok := request.(*publishOrderRequest)
	if !ok {
		return nil, fmt.Errorf("PublishOrder received invalid request type: %T", request)
	}

	return client.PublishOrder(ctx, typedRequest)
}

// NormalizeResponse returns the publish response unchanged.
func (publishOrderHandler) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// awaitConsumptionHandler binds the await step to a consumer processing attempt.
type awaitConsumptionHandler struct{}

var _ itestkit.Handler[*queueClient] = (*awaitConsumptionHandler)(nil)

// DecodeRequest decodes the request for waiting on message processing.
func (awaitConsumptionHandler) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &awaitConsumptionRequest{OrderID: ""}
	if err := decodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode AwaitConsumption request: %w", err)
	}

	return request, nil
}

// DecodeExpectedResponse decodes the expected await-step state.
func (awaitConsumptionHandler) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &awaitConsumptionResponse{State: ""}
	if len(raw) == 0 {
		return response, nil
	}
	if err := decodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode AwaitConsumption response: %w", err)
	}

	return response, nil
}

// Invoke runs consumer processing checks and returns an error until processing is complete.
func (awaitConsumptionHandler) Invoke(ctx context.Context, client *queueClient, request any) (any, error) {
	typedRequest, ok := request.(*awaitConsumptionRequest)
	if !ok {
		return nil, fmt.Errorf("AwaitConsumption received invalid request type: %T", request)
	}

	return client.AwaitConsumption(ctx, typedRequest)
}

// NormalizeResponse returns the await response unchanged.
func (awaitConsumptionHandler) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// verifyOrderHandler binds the verify step to reading data from the in-memory DB.
type verifyOrderHandler struct{}

var _ itestkit.Handler[*queueClient] = (*verifyOrderHandler)(nil)

// DecodeRequest decodes the request to verify a DB record.
func (verifyOrderHandler) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &verifyOrderRequest{OrderID: ""}
	if err := decodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode VerifyOrder request: %w", err)
	}

	return request, nil
}

// DecodeExpectedResponse decodes the expected record for assert comparison.
func (verifyOrderHandler) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &verifyOrderResponse{OrderID: "", Amount: 0, Stored: false}
	if err := decodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode VerifyOrder response: %w", err)
	}

	return response, nil
}

// Invoke reads a record from the simulated DB.
func (verifyOrderHandler) Invoke(ctx context.Context, client *queueClient, request any) (any, error) {
	typedRequest, ok := request.(*verifyOrderRequest)
	if !ok {
		return nil, fmt.Errorf("VerifyOrder received invalid request type: %T", request)
	}

	return client.VerifyOrder(ctx, typedRequest)
}

// NormalizeResponse returns the verify response unchanged.
func (verifyOrderHandler) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// decodeStrictJSON enforces strict JSON decoding for requests and expected responses.
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
