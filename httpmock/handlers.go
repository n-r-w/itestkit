package httpmock

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/n-r-w/itestkit"
)

const (
	// PlanHTTPCallsHandlerName configures planned outbound HTTP calls.
	PlanHTTPCallsHandlerName = "PlanHTTPCalls"
	// AwaitHTTPCallsHandlerName checks planned HTTP calls during an await step.
	AwaitHTTPCallsHandlerName = "AwaitHTTPCalls"
	// VerifyHTTPCallsHandlerName checks planned HTTP calls during a verify step.
	VerifyHTTPCallsHandlerName = "VerifyHTTPCalls"
)

type serverProvider interface {
	HTTPMock() *Server
}

type emptyRequest struct{}

// PlanHTTPCallsHandler stores the HTTP call plan in the case server.
type PlanHTTPCallsHandler[C serverProvider] struct{}

// AwaitHTTPCallsHandler checks the current HTTP call state.
type AwaitHTTPCallsHandler[C serverProvider] struct{}

// VerifyHTTPCallsHandler checks the final HTTP call state.
type VerifyHTTPCallsHandler[C serverProvider] struct{}

var _ itestkit.Handler[serverProvider] = PlanHTTPCallsHandler[serverProvider]{}
var _ itestkit.Handler[serverProvider] = AwaitHTTPCallsHandler[serverProvider]{}
var _ itestkit.Handler[serverProvider] = VerifyHTTPCallsHandler[serverProvider]{}

// DecodeRequest decodes a planned HTTP call set.
func (PlanHTTPCallsHandler[C]) DecodeRequest(raw json.RawMessage) (any, error) {
	var plan Plan
	if err := itestkit.DecodeStrictJSON(raw, &plan); err != nil {
		return nil, err
	}
	normalized, err := normalizePlan(plan)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

// DecodeExpectedResponse decodes the expected plan response.
func (PlanHTTPCallsHandler[C]) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return &PlanHTTPCallsResponse{}, nil
	}
	var response PlanHTTPCallsResponse
	if err := itestkit.DecodeStrictJSON(raw, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

// Invoke stores the plan in the case server.
func (PlanHTTPCallsHandler[C]) Invoke(ctx context.Context, client C, request any) (any, error) {
	plan, ok := request.(Plan)
	if !ok {
		return nil, fmt.Errorf("PlanHTTPCalls received invalid request type: %T", request)
	}
	if err := client.HTTPMock().Plan(ctx, plan); err != nil {
		return nil, err
	}
	return &PlanHTTPCallsResponse{Planned: true, ExpectedCalls: expectedTotal(plan.Calls)}, nil
}

// NormalizeResponse returns the plan response unchanged.
func (PlanHTTPCallsHandler[C]) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// DecodeRequest decodes an empty await request.
func (AwaitHTTPCallsHandler[C]) DecodeRequest(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return &emptyRequest{}, nil
	}
	var request emptyRequest
	if err := itestkit.DecodeStrictJSON(raw, &request); err != nil {
		return nil, err
	}
	return &request, nil
}

// DecodeExpectedResponse decodes the expected await response.
func (AwaitHTTPCallsHandler[C]) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return &CheckResult{}, nil
	}
	var response CheckResult
	if err := itestkit.DecodeStrictJSON(raw, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

// Invoke checks the current HTTP call state.
func (AwaitHTTPCallsHandler[C]) Invoke(ctx context.Context, client C, request any) (any, error) {
	if _, ok := request.(*emptyRequest); !ok {
		return nil, fmt.Errorf("AwaitHTTPCalls received invalid request type: %T", request)
	}
	return client.HTTPMock().Await(ctx)
}

// NormalizeResponse returns the await response unchanged.
func (AwaitHTTPCallsHandler[C]) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// DecodeRequest decodes an empty verify request.
func (VerifyHTTPCallsHandler[C]) DecodeRequest(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return &emptyRequest{}, nil
	}
	var request emptyRequest
	if err := itestkit.DecodeStrictJSON(raw, &request); err != nil {
		return nil, err
	}
	return &request, nil
}

// DecodeExpectedResponse decodes the expected verify response.
func (VerifyHTTPCallsHandler[C]) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return &CheckResult{}, nil
	}
	var response CheckResult
	if err := itestkit.DecodeStrictJSON(raw, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

// Invoke checks the final HTTP call state.
func (VerifyHTTPCallsHandler[C]) Invoke(ctx context.Context, client C, request any) (any, error) {
	if _, ok := request.(*emptyRequest); !ok {
		return nil, fmt.Errorf("VerifyHTTPCalls received invalid request type: %T", request)
	}
	return client.HTTPMock().Verify(ctx)
}

// NormalizeResponse returns the verify response unchanged.
func (VerifyHTTPCallsHandler[C]) NormalizeResponse(response any) (any, error) {
	return response, nil
}
