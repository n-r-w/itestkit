package kafkaoutbound

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/n-r-w/itestkit"
)

// sendOrderEventRequest describes action step request for outbound event publishing.
type sendOrderEventRequest struct {
	Topic   string            `json:"topic"`
	OrderID string            `json:"order_id"`
	Status  string            `json:"status"`
	Headers map[string]string `json:"headers"`
}

// sendOrderEventResponse reports publish status and resolved topic.
type sendOrderEventResponse struct {
	Published bool   `json:"published"`
	Topic     string `json:"topic"`
	OrderID   string `json:"order_id"`
}

// sendOrderEventHandler binds action step with domain publish operation.
type sendOrderEventHandler struct{}

var _ itestkit.Handler[*outboundHarness] = (*sendOrderEventHandler)(nil)

// DecodeRequest decodes action step request.
func (sendOrderEventHandler) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &sendOrderEventRequest{Topic: "", OrderID: "", Status: "", Headers: nil}
	if err := decodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode SendOrderEvent request: %w", err)
	}
	if request.Topic == "" {
		return nil, errors.New("field topic is required")
	}
	if request.OrderID == "" {
		return nil, errors.New("field order_id is required")
	}
	if request.Status == "" {
		return nil, errors.New("field status is required")
	}

	return request, nil
}

// DecodeExpectedResponse decodes expected action response.
func (sendOrderEventHandler) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &sendOrderEventResponse{Published: false, Topic: "", OrderID: ""}
	if err := decodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode SendOrderEvent response: %w", err)
	}

	return response, nil
}

// Invoke publishes outbound event through Kafka harness.
func (sendOrderEventHandler) Invoke(ctx context.Context, harness *outboundHarness, request any) (any, error) {
	typedRequest, ok := request.(*sendOrderEventRequest)
	if !ok {
		return nil, fmt.Errorf("SendOrderEvent received invalid request type: %T", request)
	}

	payload := map[string]any{
		"event":    "created",
		"order_id": typedRequest.OrderID,
		"status":   typedRequest.Status,
	}
	if err := harness.PublishJSON(ctx, typedRequest.Topic, nil, typedRequest.Headers, payload); err != nil {
		return nil, err
	}

	return &sendOrderEventResponse{
		Published: true,
		Topic:     harness.ResolveTopic(typedRequest.Topic),
		OrderID:   typedRequest.OrderID,
	}, nil
}

// NormalizeResponse returns action response unchanged.
func (sendOrderEventHandler) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// decodeStrictJSON decodes JSON with unknown fields and trailing data rejected.
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
