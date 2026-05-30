// Package itest contains ready-made preset handlers for outbound checks in the itestkit pipeline.
package itest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/n-r-w/itestkit"
	"github.com/n-r-w/itestkit/queue/probe"
)

// PlanOutboundHandlerName captures the name of the wait scheduling preset handler.
const PlanOutboundHandlerName = "PlanOutbound"

// AwaitOutboundHandlerName captures the name of the await check preset handler.
const AwaitOutboundHandlerName = "AwaitOutbound"

// VerifyOutboundHandlerName fixes the name of the verify-verification preset handler.
const VerifyOutboundHandlerName = "VerifyOutbound"

// CleanupBrokerHandlerName captures the name of the resource cleanup preset handler.
const CleanupBrokerHandlerName = "CleanupBroker"

// PlanOutboundResponse confirms that the wait was saved successfully.
type PlanOutboundResponse struct {
	Topic         string `json:"topic"`
	ExpectedCount int    `json:"expected_count"`
	Planned       bool   `json:"planned"`
}

// CleanupBrokerResponse confirms that the cleanup step completed successfully.
type CleanupBrokerResponse struct {
	Cleaned bool `json:"cleaned"`
}

// emptyRequest describes a step with no request parameters.
type emptyRequest struct{}

// PlanOutboundHandler implements the prepare step for saving the outbound plan.
type PlanOutboundHandler[C OutboundHarness] struct{}

var _ itestkit.Handler[OutboundHarness] = (*PlanOutboundHandler[OutboundHarness])(nil)

// DecodeRequest decodes the outbound message expectation contract.
func (PlanOutboundHandler[C]) DecodeRequest(raw json.RawMessage) (any, error) {
	expectation := probe.OutboundExpectation{
		Topic:         "",
		Key:           nil,
		Headers:       nil,
		HeadersMode:   "",
		ExpectedCount: 0,
		Payload:       nil,
		PayloadSubset: nil,
		Ordering:      "",
	}
	if err := itestkit.DecodeStrictJSON(raw, &expectation); err != nil {
		return nil, fmt.Errorf("decode PlanOutbound request: %w", err)
	}

	normalized, err := probe.NormalizeExpectation(expectation)
	if err != nil {
		return nil, fmt.Errorf("validate PlanOutbound request: %w", err)
	}

	return normalized, nil
}

// DecodeExpectedResponse decodes the expected response from the prepare step.
func (PlanOutboundHandler[C]) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &PlanOutboundResponse{Topic: "", ExpectedCount: 0, Planned: false}
	if len(bytes.TrimSpace(raw)) == 0 {
		return response, nil
	}
	if err := itestkit.DecodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode PlanOutbound response: %w", err)
	}

	return response, nil
}

// Invoke stores the contract in the harness and activates the surveillance boundary.
func (PlanOutboundHandler[C]) Invoke(ctx context.Context, harness C, request any) (any, error) {
	expectation, ok := request.(probe.OutboundExpectation)
	if !ok {
		return nil, fmt.Errorf("PlanOutbound received invalid request type: %T", request)
	}

	if err := harness.PlanOutbound(ctx, expectation); err != nil {
		return nil, err
	}

	return &PlanOutboundResponse{
		Topic:         expectation.Topic,
		ExpectedCount: expectation.ExpectedCount,
		Planned:       true,
	}, nil
}

// NormalizeResponse returns the response unchanged.
func (PlanOutboundHandler[C]) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// AwaitOutboundHandler implements an await step to observe an outbound publication.
type AwaitOutboundHandler[C OutboundHarness] struct{}

var _ itestkit.Handler[OutboundHarness] = (*AwaitOutboundHandler[OutboundHarness])(nil)

// DecodeRequest decodes an empty await step request.
func (AwaitOutboundHandler[C]) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &emptyRequest{}
	if len(bytes.TrimSpace(raw)) == 0 {
		return request, nil
	}
	if err := itestkit.DecodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode AwaitOutbound request: %w", err)
	}

	return request, nil
}

// DecodeExpectedResponse decodes the awaited result.
func (AwaitOutboundHandler[C]) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &probe.CheckResult{MatchedCount: 0, ObservedMessages: nil, LastMismatchReason: ""}
	if len(bytes.TrimSpace(raw)) == 0 {
		return response, nil
	}
	if err := itestkit.DecodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode AwaitOutbound response: %w", err)
	}

	return response, nil
}

// Invoke runs an intermediate await check via a harness.
func (AwaitOutboundHandler[C]) Invoke(ctx context.Context, harness C, request any) (any, error) {
	if _, ok := request.(*emptyRequest); !ok {
		return nil, fmt.Errorf("AwaitOutbound received invalid request type: %T", request)
	}

	result, err := harness.AwaitOutbound(ctx)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// NormalizeResponse returns the response unchanged.
func (AwaitOutboundHandler[C]) NormalizeResponse(response any) (any, error) {
	return normalizeProbeCheckResult(response)
}

// VerifyOutboundHandler implements the verify step for final verification of the outbound publication.
type VerifyOutboundHandler[C OutboundHarness] struct{}

var _ itestkit.Handler[OutboundHarness] = (*VerifyOutboundHandler[OutboundHarness])(nil)

// DecodeRequest decodes an empty verify step request.
func (VerifyOutboundHandler[C]) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &emptyRequest{}
	if len(bytes.TrimSpace(raw)) == 0 {
		return request, nil
	}
	if err := itestkit.DecodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode VerifyOutbound request: %w", err)
	}

	return request, nil
}

// DecodeExpectedResponse decodes the expected verify result.
func (VerifyOutboundHandler[C]) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &probe.CheckResult{MatchedCount: 0, ObservedMessages: nil, LastMismatchReason: ""}
	if len(bytes.TrimSpace(raw)) == 0 {
		return response, nil
	}
	if err := itestkit.DecodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode VerifyOutbound response: %w", err)
	}

	return response, nil
}

// Invoke runs the final verify check via the harness.
func (VerifyOutboundHandler[C]) Invoke(ctx context.Context, harness C, request any) (any, error) {
	if _, ok := request.(*emptyRequest); !ok {
		return nil, fmt.Errorf("VerifyOutbound received invalid request type: %T", request)
	}

	result, err := harness.VerifyOutbound(ctx)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// NormalizeResponse returns the response unchanged.
func (VerifyOutboundHandler[C]) NormalizeResponse(response any) (any, error) {
	return normalizeProbeCheckResult(response)
}

// CleanupBrokerHandler implements the cleanup step to free resources.
type CleanupBrokerHandler[C OutboundHarness] struct{}

var _ itestkit.Handler[OutboundHarness] = (*CleanupBrokerHandler[OutboundHarness])(nil)

// DecodeRequest decodes an empty cleanup request.
func (CleanupBrokerHandler[C]) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &emptyRequest{}
	if len(bytes.TrimSpace(raw)) == 0 {
		return request, nil
	}
	if err := itestkit.DecodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode CleanupBroker request: %w", err)
	}

	return request, nil
}

// DecodeExpectedResponse decodes the expected cleanup response.
func (CleanupBrokerHandler[C]) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &CleanupBrokerResponse{Cleaned: false}
	if len(bytes.TrimSpace(raw)) == 0 {
		return response, nil
	}
	if err := itestkit.DecodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode CleanupBroker response: %w", err)
	}

	return response, nil
}

// Invoke releases the broker helper's resources.
func (CleanupBrokerHandler[C]) Invoke(ctx context.Context, harness C, request any) (any, error) {
	if _, ok := request.(*emptyRequest); !ok {
		return nil, fmt.Errorf("CleanupBroker received invalid request type: %T", request)
	}

	if err := harness.CleanupBroker(ctx); err != nil {
		return nil, err
	}

	return &CleanupBrokerResponse{Cleaned: true}, nil
}

// NormalizeResponse returns the response unchanged.
func (CleanupBrokerHandler[C]) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// normalizeProbeCheckResult brings CheckResult to a stable form for assert comparison.
func normalizeProbeCheckResult(response any) (probe.CheckResult, error) {
	result, err := castProbeCheckResult(response)
	if err != nil {
		return probe.CheckResult{}, err
	}

	normalizedMessages := make([]probe.Message, 0, len(result.ObservedMessages))
	for _, message := range result.ObservedMessages {
		normalizedPayload, payloadErr := canonicalizeRawJSON(message.Payload)
		if payloadErr != nil {
			return probe.CheckResult{}, fmt.Errorf("normalize observed payload: %w", payloadErr)
		}

		normalizedMessages = append(normalizedMessages, probe.Message{
			Topic:   message.Topic,
			Key:     message.Key,
			Headers: message.Headers,
			Payload: normalizedPayload,
			Offset:  message.Offset,
		})
	}

	sort.Slice(normalizedMessages, func(left, right int) bool {
		if normalizedMessages[left].Topic != normalizedMessages[right].Topic {
			return normalizedMessages[left].Topic < normalizedMessages[right].Topic
		}
		return normalizedMessages[left].Offset < normalizedMessages[right].Offset
	})

	return probe.CheckResult{
		MatchedCount:       result.MatchedCount,
		ObservedMessages:   normalizedMessages,
		LastMismatchReason: result.LastMismatchReason,
	}, nil
}

// castProbeCheckResult supports a normalizer for the pointer/value representation of CheckResult.
func castProbeCheckResult(response any) (probe.CheckResult, error) {
	typedResult, ok := response.(probe.CheckResult)
	if ok {
		return typedResult, nil
	}

	typedResultPtr, ok := response.(*probe.CheckResult)
	if ok && typedResultPtr != nil {
		return *typedResultPtr, nil
	}

	return probe.CheckResult{}, fmt.Errorf("invalid check result type: %T", response)
}

// canonicalizeRawJSON formats raw JSON into deterministic form.
func canonicalizeRawJSON(raw json.RawMessage) (json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}

	var payload any
	if err := itestkit.DecodeStrictJSON(raw, &payload); err != nil {
		return nil, err
	}

	normalized, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal json: %w", err)
	}

	return normalized, nil
}
