package custom

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/n-r-w/itestkit"
)

// setPrefixHandler binds step JSON to the client's SetPrefix method.
type setPrefixHandler struct{}

var _ itestkit.Handler[*echoClient] = (*setPrefixHandler)(nil)

// DecodeRequest builds the request for prepare steps.
func (setPrefixHandler) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &setPrefixRequest{Prefix: ""}
	if err := decodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode SetPrefix request: %w", err)
	}
	return request, nil
}

// DecodeExpectedResponse prepares the expected response, even if it is unused.
func (setPrefixHandler) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &setPrefixResponse{}
	if len(raw) == 0 {
		return response, nil
	}
	if err := decodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode SetPrefix response: %w", err)
	}
	return response, nil
}

// Invoke calls the client method and validates request type.
func (setPrefixHandler) Invoke(ctx context.Context, client *echoClient, request any) (any, error) {
	typedRequest, ok := request.(*setPrefixRequest)
	if !ok {
		return nil, fmt.Errorf("SetPrefix received invalid request type: %T", request)
	}
	return client.SetPrefix(ctx, typedRequest)
}

// NormalizeResponse returns the response unchanged because it is already stable.
func (setPrefixHandler) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// echoHandler binds JSON to the Echo action step.
type echoHandler struct{}

var _ itestkit.Handler[*echoClient] = (*echoHandler)(nil)

// DecodeRequest prepares the request for Echo.
func (echoHandler) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &echoRequest{Message: ""}
	if err := decodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode Echo request: %w", err)
	}
	return request, nil
}

// DecodeExpectedResponse prepares the expected response for comparison.
func (echoHandler) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &echoResponse{Message: ""}
	if err := decodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode Echo response: %w", err)
	}
	return response, nil
}

// Invoke executes the action call and validates input request type.
func (echoHandler) Invoke(ctx context.Context, client *echoClient, request any) (any, error) {
	typedRequest, ok := request.(*echoRequest)
	if !ok {
		return nil, fmt.Errorf("Echo received invalid request type: %T", request)
	}
	return client.Echo(ctx, typedRequest)
}

// NormalizeResponse returns Echo response as a JSON-like object for exact and partial comparison.
func (echoHandler) NormalizeResponse(response any) (any, error) {
	typedResponse, ok := response.(*echoResponse)
	if !ok {
		return nil, fmt.Errorf("Echo received invalid response type: %T", response)
	}
	return map[string]any{"message": typedResponse.Message}, nil
}

// publishEventHandler binds the publish step to event publication in the mock event flow.
type publishEventHandler struct{}

var _ itestkit.Handler[*echoClient] = (*publishEventHandler)(nil)

// DecodeRequest decodes the publish request JSON.
func (publishEventHandler) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &publishEventRequest{Message: ""}
	if err := decodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode PublishEvent request: %w", err)
	}

	return request, nil
}

// DecodeExpectedResponse decodes the expected publish-step result.
func (publishEventHandler) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &publishEventResponse{EventID: ""}
	if len(raw) == 0 {
		return response, nil
	}

	if err := decodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode PublishEvent response: %w", err)
	}

	return response, nil
}

// Invoke publishes an event through the client.
func (publishEventHandler) Invoke(ctx context.Context, client *echoClient, request any) (any, error) {
	typedRequest, ok := request.(*publishEventRequest)
	if !ok {
		return nil, fmt.Errorf("PublishEvent received invalid request type: %T", request)
	}

	return client.PublishEvent(ctx, typedRequest)
}

// NormalizeResponse returns the publish response unchanged.
func (publishEventHandler) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// awaitProcessedHandler binds the await step to polling processing state.
type awaitProcessedHandler struct{}

var _ itestkit.Handler[*echoClient] = (*awaitProcessedHandler)(nil)

// DecodeRequest decodes await polling parameters.
func (awaitProcessedHandler) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &awaitProcessedRequest{EventID: ""}
	if err := decodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode AwaitProcessed request: %w", err)
	}

	return request, nil
}

// DecodeExpectedResponse decodes the expected await-step state.
func (awaitProcessedHandler) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &awaitProcessedResponse{State: ""}
	if len(raw) == 0 {
		return response, nil
	}

	if err := decodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode AwaitProcessed response: %w", err)
	}

	return response, nil
}

// Invoke polls processing state and returns an error until ready.
func (awaitProcessedHandler) Invoke(ctx context.Context, client *echoClient, request any) (any, error) {
	typedRequest, ok := request.(*awaitProcessedRequest)
	if !ok {
		return nil, fmt.Errorf("AwaitProcessed received invalid request type: %T", request)
	}

	return client.AwaitProcessed(ctx, typedRequest)
}

// NormalizeResponse returns the await response unchanged.
func (awaitProcessedHandler) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// verifyStateHandler binds the verify step to processed-event state validation.
type verifyStateHandler struct{}

var _ itestkit.Handler[*echoClient] = (*verifyStateHandler)(nil)

// DecodeRequest decodes marker-aware state expectations for side-effect verification.
func (verifyStateHandler) DecodeRequest(raw json.RawMessage) (any, error) {
	requestJSON := verifyStateRequestJSON{Expected: json.RawMessage(`{}`)}
	if len(raw) > 0 {
		if err := decodeStrictJSON(raw, &requestJSON); err != nil {
			return nil, fmt.Errorf("decode VerifyState request: %w", err)
		}
	}

	expected, err := itestkit.DecodeExpectedJSON(requestJSON.Expected)
	if err != nil {
		return nil, fmt.Errorf("decode VerifyState expected state: %w", err)
	}

	return &verifyStateRequest{Expected: expected}, nil
}

// DecodeExpectedResponse decodes the expected verify state.
func (verifyStateHandler) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &verifyStateResponse{State: "", EventID: ""}
	if err := decodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode VerifyState response: %w", err)
	}

	return response, nil
}

// Invoke fetches final event-flow state and compares it with the request expectation.
func (verifyStateHandler) Invoke(ctx context.Context, client *echoClient, request any) (any, error) {
	typedRequest, ok := request.(*verifyStateRequest)
	if !ok {
		return nil, fmt.Errorf("VerifyState received invalid request type: %T", request)
	}

	response, err := client.VerifyState(ctx, typedRequest)
	if err != nil {
		return nil, err
	}

	actualState := map[string]any{
		"state":    response.State,
		"event_id": response.EventID,
	}
	matchErr := itestkit.MatchExpectedJSON(typedRequest.Expected, actualState, itestkit.MatchModeExact)
	if matchErr != nil {
		return nil, fmt.Errorf("verify state expectation: %w", matchErr)
	}

	return response, nil
}

// NormalizeResponse returns the verify response unchanged.
func (verifyStateHandler) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// decodeStrictJSON enforces strict JSON parsing for requests and responses.
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
