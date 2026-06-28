package httpmockexample

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type createOrderRequest struct {
	OrderID string `json:"order_id"`
	Amount  int    `json:"amount"`
}

type createOrderResponse struct {
	OrderID      string `json:"order_id"`
	External     string `json:"external"`
	ApprovalCode string `json:"approval_code"`
}

type chargeRequest struct {
	OrderID string `json:"order_id"`
	Amount  int    `json:"amount"`
}

type chargeResponse struct {
	Result       string `json:"result"`
	ApprovalCode string `json:"approval_code"`
}

type orderClient struct {
	baseURL string
	client  *http.Client
}

// newOrderClient creates a client that calls the external payment API.
func newOrderClient(baseURL string) *orderClient {
	return &orderClient{baseURL: strings.TrimRight(baseURL, "/"), client: http.DefaultClient}
}

// CreateOrder calls the external charge endpoint and returns the order result.
func (client *orderClient) CreateOrder(ctx context.Context, request createOrderRequest) (createOrderResponse, error) {
	chargeBody, err := json.Marshal(chargeRequest(request))
	if err != nil {
		return createOrderResponse{}, fmt.Errorf("marshal charge request: %w", err)
	}

	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		client.baseURL+"/charges?dry_run=false",
		bytes.NewReader(chargeBody),
	)
	if err != nil {
		return createOrderResponse{}, fmt.Errorf("create charge request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("X-Request-ID", request.OrderID)
	httpRequest.Header.Set("X-Timestamp", fmt.Sprintf("order-%s", request.OrderID))
	httpRequest.Header.Set("X-Signature", fmt.Sprintf("signed-%s-%d", request.OrderID, request.Amount))

	response, err := client.client.Do(httpRequest)
	if err != nil {
		return createOrderResponse{}, fmt.Errorf("call charge endpoint: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		if closeErr := response.Body.Close(); closeErr != nil {
			return createOrderResponse{}, fmt.Errorf("close charge response: %w", closeErr)
		}
		return createOrderResponse{}, statusError{code: statusFailed, message: "charge endpoint returned error"}
	}

	var charge chargeResponse
	decodeErr := json.NewDecoder(response.Body).Decode(&charge)
	if closeErr := response.Body.Close(); closeErr != nil {
		return createOrderResponse{}, fmt.Errorf("close charge response: %w", closeErr)
	}
	if decodeErr != nil {
		return createOrderResponse{}, fmt.Errorf("decode charge response: %w", decodeErr)
	}

	return createOrderResponse{
		OrderID:      request.OrderID,
		External:     charge.Result,
		ApprovalCode: charge.ApprovalCode,
	}, nil
}
