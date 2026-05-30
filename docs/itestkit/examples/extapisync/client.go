package extapisync

import (
	"context"
	"errors"
	"fmt"
)

// seedOrder describes one record to preload into the "DB" before the action step.
type seedOrder struct {
	OrderID string `json:"order_id"`
	Amount  int    `json:"amount"`
}

// seedDataRequest contains records for state-initialization prepare steps.
type seedDataRequest struct {
	Orders []seedOrder `json:"orders"`
}

// seedDataResponse confirms how many records were prepared in the "DB".
type seedDataResponse struct {
	StoredOrders int `json:"stored_orders"`
}

// externalChargeRequest models payload sent to an external API.
type externalChargeRequest struct {
	OrderID string `json:"order_id"`
	Amount  int    `json:"amount"`
}

// externalChargeResponse models the external API response.
type externalChargeResponse struct {
	Result       string `json:"result"`
	ApprovalCode string `json:"approval_code"`
}

// planExternalChargeRequest defines expectations and a stub response for the external API.
type planExternalChargeRequest struct {
	ExpectedRequest externalChargeRequest  `json:"expected_request"`
	StubResponse    externalChargeResponse `json:"stub_response"`
}

// planExternalChargeResponse confirms that the external call plan is stored.
type planExternalChargeResponse struct {
	Planned bool `json:"planned"`
}

// createOrderRequest describes "our API" input that triggers an external call.
type createOrderRequest struct {
	OrderID string `json:"order_id"`
}

// createOrderResponse describes final "our API" output after a synchronous external call.
type createOrderResponse struct {
	OrderID        string `json:"order_id"`
	ExternalResult string `json:"external_result"`
	ApprovalCode   string `json:"approval_code"`
}

// verifyExternalChargeRequest defines verification of external call fact and parameters.
type verifyExternalChargeRequest struct {
	OrderID string `json:"order_id"`
}

// verifyExternalChargeResponse returns observed external interaction state.
type verifyExternalChargeResponse struct {
	Called      bool                  `json:"called"`
	CallsCount  int                   `json:"calls_count"`
	LastRequest externalChargeRequest `json:"last_request"`
}

// syncOrderRecord models a record in in-memory order storage.
type syncOrderRecord struct {
	OrderID        string
	Amount         int
	ExternalResult string
	ApprovalCode   string
}

// externalChargeExpectation stores expectations for one external call.
type externalChargeExpectation struct {
	Planned         bool
	ExpectedRequest externalChargeRequest
	StubResponse    externalChargeResponse
}

// syncClient stores in-memory state of "our API" and the external API mock.
type syncClient struct {
	orders       map[string]syncOrderRecord
	expectation  externalChargeExpectation
	externalCall []externalChargeRequest
}

// newSyncClient creates clean state for isolated case execution.
func newSyncClient() *syncClient {
	return &syncClient{
		orders: make(map[string]syncOrderRecord),
		expectation: externalChargeExpectation{
			Planned:         false,
			ExpectedRequest: externalChargeRequest{OrderID: "", Amount: 0},
			StubResponse:    externalChargeResponse{Result: "", ApprovalCode: ""},
		},
		externalCall: make([]externalChargeRequest, 0),
	}
}

// SeedData prepares the in-memory "DB" with test-scenario data.
func (client *syncClient) SeedData(_ context.Context, request *seedDataRequest) (*seedDataResponse, error) {
	client.orders = make(map[string]syncOrderRecord, len(request.Orders))
	client.expectation = externalChargeExpectation{
		Planned:         false,
		ExpectedRequest: externalChargeRequest{OrderID: "", Amount: 0},
		StubResponse:    externalChargeResponse{Result: "", ApprovalCode: ""},
	}
	client.externalCall = client.externalCall[:0]

	for _, order := range request.Orders {
		client.orders[order.OrderID] = syncOrderRecord{
			OrderID:        order.OrderID,
			Amount:         order.Amount,
			ExternalResult: "",
			ApprovalCode:   "",
		}
	}

	return &seedDataResponse{StoredOrders: len(request.Orders)}, nil
}

// PlanExternalCharge stores the expected outbound request and external API stub response.
func (client *syncClient) PlanExternalCharge(
	_ context.Context,
	request *planExternalChargeRequest,
) (*planExternalChargeResponse, error) {
	client.expectation = externalChargeExpectation{
		Planned:         true,
		ExpectedRequest: request.ExpectedRequest,
		StubResponse:    request.StubResponse,
	}
	client.externalCall = client.externalCall[:0]

	return &planExternalChargeResponse{Planned: true}, nil
}

// CreateOrder simulates an "our API" endpoint that synchronously calls the external API.
func (client *syncClient) CreateOrder(_ context.Context, request *createOrderRequest) (*createOrderResponse, error) {
	order, exists := client.orders[request.OrderID]
	if !exists {
		return nil, syncStatusError{
			code:    syncStatusFailed,
			message: fmt.Sprintf("order %q is not found", request.OrderID),
		}
	}

	externalRequest := externalChargeRequest{OrderID: order.OrderID, Amount: order.Amount}
	externalResponse, err := client.invokeExternalCharge(externalRequest)
	if err != nil {
		return nil, syncStatusError{
			code:    syncStatusFailed,
			message: fmt.Sprintf("external charge failed: %v", err),
		}
	}

	order.ExternalResult = externalResponse.Result
	order.ApprovalCode = externalResponse.ApprovalCode
	client.orders[order.OrderID] = order

	return &createOrderResponse{
		OrderID:        order.OrderID,
		ExternalResult: order.ExternalResult,
		ApprovalCode:   order.ApprovalCode,
	}, nil
}

// VerifyExternalCharge confirms that the external API was called with expected data.
func (client *syncClient) VerifyExternalCharge(
	_ context.Context,
	request *verifyExternalChargeRequest,
) (*verifyExternalChargeResponse, error) {
	if len(client.externalCall) == 0 {
		return nil, syncStatusError{
			code:    syncStatusFailed,
			message: "external API was not called",
		}
	}

	lastRequest := client.externalCall[len(client.externalCall)-1]
	if lastRequest.OrderID != request.OrderID {
		return nil, syncStatusError{
			code:    syncStatusFailed,
			message: fmt.Sprintf("unexpected external order id: got %q", lastRequest.OrderID),
		}
	}

	return &verifyExternalChargeResponse{
		Called:      true,
		CallsCount:  len(client.externalCall),
		LastRequest: lastRequest,
	}, nil
}

// invokeExternalCharge validates request match and returns the preconfigured stub response.
func (client *syncClient) invokeExternalCharge(
	request externalChargeRequest,
) (externalChargeResponse, error) {
	if !client.expectation.Planned {
		return externalChargeResponse{}, errors.New("external expectation is not planned")
	}
	if request != client.expectation.ExpectedRequest {
		return externalChargeResponse{}, fmt.Errorf(
			"external request mismatch: want %+v got %+v",
			client.expectation.ExpectedRequest,
			request,
		)
	}

	client.externalCall = append(client.externalCall, request)

	return client.expectation.StubResponse, nil
}
