package custom

import (
	"context"
	"encoding/json"
)

const (
	// awaitProcessedSuccessAttempt defines the await polling attempt when processing completes successfully.
	awaitProcessedSuccessAttempt = 2
	// customStatePending marks an event that is not processed yet.
	customStatePending = "pending"
	// customStateProcessed marks an event that has been processed.
	customStateProcessed = "processed"
)

// setPrefixRequest describes a prefix update for client messages.
type setPrefixRequest struct {
	Prefix string `json:"prefix"`
}

// setPrefixResponse confirms that the prefix was applied in a prepare step.
type setPrefixResponse struct{}

// echoRequest describes the input message for the Echo handler.
type echoRequest struct {
	Message string `json:"message"`
}

// echoResponse contains the final response used in assert.
type echoResponse struct {
	Message string `json:"message"`
}

// publishEventRequest describes an event published to the mock event flow.
type publishEventRequest struct {
	Message string `json:"message"`
}

// publishEventResponse returns the published event ID.
type publishEventResponse struct {
	EventID string `json:"event_id"`
}

// awaitProcessedRequest describes waiting for a previously published event to be processed.
type awaitProcessedRequest struct {
	EventID string `json:"event_id"`
}

// awaitProcessedResponse reflects the current processing state of the event.
type awaitProcessedResponse struct {
	State string `json:"state"`
}

// verifyStateRequest contains a marker-aware expectation for final mock event-flow state.
type verifyStateRequest struct {
	Expected any
}

// verifyStateRequestJSON keeps marker templates as raw JSON until matcher-aware decoding.
type verifyStateRequestJSON struct {
	Expected json.RawMessage `json:"expected"`
}

// verifyStateResponse contains normalized state for assert.
type verifyStateResponse struct {
	State   string `json:"state"`
	EventID string `json:"event_id"`
}

// echoClient stores mutable state updated by case steps.
type echoClient struct {
	prefix       string
	publishedID  string
	processed    bool
	awaitAttempt int
}

// SetPrefix updates client state so the next action step sees the required prefix.
func (client *echoClient) SetPrefix(_ context.Context, request *setPrefixRequest) (*setPrefixResponse, error) {
	client.prefix = request.Prefix
	return &setPrefixResponse{}, nil
}

// Echo returns the message with prefix to validate arrange logic.
func (client *echoClient) Echo(_ context.Context, request *echoRequest) (*echoResponse, error) {
	return &echoResponse{Message: client.prefix + request.Message}, nil
}

// PublishEvent records the event and resets processing state for the next await.
func (client *echoClient) PublishEvent(_ context.Context, request *publishEventRequest) (*publishEventResponse, error) {
	client.publishedID = request.Message
	client.processed = false
	client.awaitAttempt = 0

	return &publishEventResponse{EventID: request.Message}, nil
}

// AwaitProcessed simulates eventual consistency: first call is pending, second is processed.
func (client *echoClient) AwaitProcessed(
	_ context.Context,
	request *awaitProcessedRequest,
) (*awaitProcessedResponse, error) {
	if client.publishedID == "" || request.EventID != client.publishedID {
		return &awaitProcessedResponse{State: "missing"}, statusError{
			code:    statusFailed,
			message: "event is not published",
		}
	}

	client.awaitAttempt++
	if client.awaitAttempt < awaitProcessedSuccessAttempt {
		return &awaitProcessedResponse{State: customStatePending}, statusError{
			code:    statusFailed,
			message: "event is not processed yet",
		}
	}

	client.processed = true

	return &awaitProcessedResponse{State: customStateProcessed}, nil
}

// VerifyState returns the final state validated in assert.
func (client *echoClient) VerifyState(_ context.Context, _ *verifyStateRequest) (*verifyStateResponse, error) {
	state := customStatePending
	if client.processed {
		state = customStateProcessed
	}

	return &verifyStateResponse{State: state, EventID: client.publishedID}, nil
}
