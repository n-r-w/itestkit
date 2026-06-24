package extapiasync

import (
	"context"
	"errors"
	"fmt"
	"slices"
)

const (
	// asyncProcessReadyAttempt defines the await polling attempt when processing completes successfully.
	asyncProcessReadyAttempt = 2
	// asyncOrderStatePending marks an order waiting for external synchronization.
	asyncOrderStatePending = "pending"
	// asyncOrderStateProcessed marks an order synchronized with the external system.
	asyncOrderStateProcessed = "processed"
)

// seedOrder describes a record to preload into the in-memory "DB".
type seedOrder struct {
	OrderID string `json:"order_id"`
	Amount  int    `json:"amount"`
}

// seedDataRequest provides initial data for the prepare step.
type seedDataRequest struct {
	Orders []seedOrder `json:"orders"`
}

// seedDataResponse confirms how many records were loaded.
type seedDataResponse struct {
	StoredOrders int `json:"stored_orders"`
}

// externalChargeRequest models the outbound request payload to an external API.
type externalChargeRequest struct {
	OrderID string `json:"order_id"`
	Amount  int    `json:"amount"`
}

// externalChargeResponse models the external API response.
type externalChargeResponse struct {
	Result       string `json:"result"`
	ApprovalCode string `json:"approval_code"`
}

// planExternalChargeRequest contains the expected external request and stub response.
type planExternalChargeRequest struct {
	ExpectedRequest externalChargeRequest  `json:"expected_request"`
	StubResponse    externalChargeResponse `json:"stub_response"`
}

// planExternalChargeResponse confirms that the external call plan is stored.
type planExternalChargeResponse struct {
	Planned bool `json:"planned"`
}

// submitOrderRequest describes a call to "our API" that starts async processing.
type submitOrderRequest struct {
	OrderID string `json:"order_id"`
}

// submitOrderResponse shows that the order was accepted for processing.
type submitOrderResponse struct {
	OrderID string `json:"order_id"`
	State   string `json:"state"`
}

// awaitExternalSyncRequest defines a check for async external sync completion.
type awaitExternalSyncRequest struct {
	OrderID string `json:"order_id"`
}

// awaitExternalSyncResponse reflects the current async process state.
type awaitExternalSyncResponse struct {
	OrderID  string `json:"order_id"`
	State    string `json:"state"`
	Attempts int    `json:"attempts"`
}

// verifyExternalChargeRequest defines verification of outbound call fact and payload.
type verifyExternalChargeRequest struct {
	OrderID string `json:"order_id"`
}

// verifyExternalChargeResponse returns diagnostics for the executed outbound call.
type verifyExternalChargeResponse struct {
	Called      bool                  `json:"called"`
	CallsCount  int                   `json:"calls_count"`
	LastRequest externalChargeRequest `json:"last_request"`
}

// getOrderRequest defines fetching the current order state from "our API".
type getOrderRequest struct {
	OrderID string `json:"order_id"`
}

// getOrderResponse contains the final order state after async processing.
type getOrderResponse struct {
	OrderID        string `json:"order_id"`
	Processed      bool   `json:"processed"`
	ExternalResult string `json:"external_result"`
	ApprovalCode   string `json:"approval_code"`
}

// asyncOrderRecord models an order record in in-memory storage.
type asyncOrderRecord struct {
	OrderID        string
	Amount         int
	Processed      bool
	ExternalResult string
	ApprovalCode   string
}

// externalChargeExpectation stores expectations for the external request.
type externalChargeExpectation struct {
	Planned         bool
	ExpectedRequest externalChargeRequest
	StubResponse    externalChargeResponse
}

// asyncClient stores state for "our API", task queue, and external API mock.
type asyncClient struct {
	orders         map[string]asyncOrderRecord
	expectation    externalChargeExpectation
	externalCalls  []externalChargeRequest
	workerAttempts map[string]int
	pendingOrderID []string
}

// newAsyncClient creates a clean harness for one case.
func newAsyncClient() *asyncClient {
	return &asyncClient{
		orders: make(map[string]asyncOrderRecord),
		expectation: externalChargeExpectation{
			Planned:         false,
			ExpectedRequest: externalChargeRequest{OrderID: "", Amount: 0},
			StubResponse:    externalChargeResponse{Result: "", ApprovalCode: ""},
		},
		externalCalls:  make([]externalChargeRequest, 0),
		workerAttempts: make(map[string]int),
		pendingOrderID: make([]string, 0),
	}
}

// SeedData loads initial data and resets async pipeline state.
func (client *asyncClient) SeedData(_ context.Context, request *seedDataRequest) (*seedDataResponse, error) {
	client.orders = make(map[string]asyncOrderRecord, len(request.Orders))
	client.expectation = externalChargeExpectation{
		Planned:         false,
		ExpectedRequest: externalChargeRequest{OrderID: "", Amount: 0},
		StubResponse:    externalChargeResponse{Result: "", ApprovalCode: ""},
	}
	client.externalCalls = client.externalCalls[:0]
	client.workerAttempts = make(map[string]int)
	client.pendingOrderID = client.pendingOrderID[:0]

	for _, order := range request.Orders {
		client.orders[order.OrderID] = asyncOrderRecord{
			OrderID:        order.OrderID,
			Amount:         order.Amount,
			Processed:      false,
			ExternalResult: "",
			ApprovalCode:   "",
		}
	}

	return &seedDataResponse{StoredOrders: len(request.Orders)}, nil
}

// PlanExternalCharge stores the expected external request and stub response.
func (client *asyncClient) PlanExternalCharge(
	_ context.Context,
	request *planExternalChargeRequest,
) (*planExternalChargeResponse, error) {
	client.expectation = externalChargeExpectation{
		Planned:         true,
		ExpectedRequest: request.ExpectedRequest,
		StubResponse:    request.StubResponse,
	}
	client.externalCalls = client.externalCalls[:0]

	return &planExternalChargeResponse{Planned: true}, nil
}

// SubmitOrder simulates an "our API" endpoint that queues the order for async processing.
func (client *asyncClient) SubmitOrder(_ context.Context, request *submitOrderRequest) (*submitOrderResponse, error) {
	if _, exists := client.orders[request.OrderID]; !exists {
		return nil, asyncStatusError{
			code:    asyncStatusFailed,
			message: fmt.Sprintf("order %q is not found", request.OrderID),
		}
	}

	client.enqueuePendingOrder(request.OrderID)

	return &submitOrderResponse{OrderID: request.OrderID, State: "queued"}, nil
}

// AwaitExternalSync executes the await step and completes async processing on the target attempt.
func (client *asyncClient) AwaitExternalSync(
	_ context.Context,
	request *awaitExternalSyncRequest,
) (*awaitExternalSyncResponse, error) {
	order, exists := client.orders[request.OrderID]
	if !exists {
		return nil, asyncStatusError{
			code:    asyncStatusFailed,
			message: fmt.Sprintf("order %q is not found", request.OrderID),
		}
	}
	if order.Processed {
		return &awaitExternalSyncResponse{
			OrderID:  request.OrderID,
			State:    asyncOrderStateProcessed,
			Attempts: client.workerAttempts[request.OrderID],
		}, nil
	}
	if !client.isPendingOrder(request.OrderID) {
		return &awaitExternalSyncResponse{
				OrderID:  request.OrderID,
				State:    "missing",
				Attempts: client.workerAttempts[request.OrderID],
			}, asyncStatusError{
				code:    asyncStatusFailed,
				message: "order is not queued for processing",
			}
	}

	client.workerAttempts[request.OrderID]++
	attempt := client.workerAttempts[request.OrderID]
	if attempt < asyncProcessReadyAttempt {
		return &awaitExternalSyncResponse{
				OrderID:  request.OrderID,
				State:    asyncOrderStatePending,
				Attempts: attempt,
			}, asyncStatusError{
				code:    asyncStatusFailed,
				message: "external sync is not completed yet",
			}
	}

	externalRequest := externalChargeRequest{OrderID: order.OrderID, Amount: order.Amount}
	externalResponse, err := client.invokeExternalCharge(externalRequest)
	if err != nil {
		return &awaitExternalSyncResponse{
				OrderID:  request.OrderID,
				State:    asyncOrderStatePending,
				Attempts: attempt,
			}, asyncStatusError{
				code:    asyncStatusFailed,
				message: fmt.Sprintf("external charge failed: %v", err),
			}
	}

	order.Processed = true
	order.ExternalResult = externalResponse.Result
	order.ApprovalCode = externalResponse.ApprovalCode
	client.orders[order.OrderID] = order
	client.removePendingOrder(order.OrderID)

	return &awaitExternalSyncResponse{
		OrderID:  request.OrderID,
		State:    asyncOrderStateProcessed,
		Attempts: attempt,
	}, nil
}

// VerifyExternalCharge confirms that the outbound call was made for the expected order.
func (client *asyncClient) VerifyExternalCharge(
	_ context.Context,
	request *verifyExternalChargeRequest,
) (*verifyExternalChargeResponse, error) {
	if len(client.externalCalls) == 0 {
		return nil, asyncStatusError{
			code:    asyncStatusFailed,
			message: "external API was not called",
		}
	}

	lastRequest := client.externalCalls[len(client.externalCalls)-1]
	if lastRequest.OrderID != request.OrderID {
		return nil, asyncStatusError{
			code:    asyncStatusFailed,
			message: fmt.Sprintf("unexpected external order id: got %q", lastRequest.OrderID),
		}
	}

	return &verifyExternalChargeResponse{
		Called:      true,
		CallsCount:  len(client.externalCalls),
		LastRequest: lastRequest,
	}, nil
}

// GetOrder returns current order state for final assert.response.
func (client *asyncClient) GetOrder(_ context.Context, request *getOrderRequest) (*getOrderResponse, error) {
	order, exists := client.orders[request.OrderID]
	if !exists {
		return nil, asyncStatusError{
			code:    asyncStatusFailed,
			message: fmt.Sprintf("order %q is not found", request.OrderID),
		}
	}

	return &getOrderResponse{
		OrderID:        order.OrderID,
		Processed:      order.Processed,
		ExternalResult: order.ExternalResult,
		ApprovalCode:   order.ApprovalCode,
	}, nil
}

// invokeExternalCharge validates outbound request payload and returns the planned stub response.
func (client *asyncClient) invokeExternalCharge(
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

	client.externalCalls = append(client.externalCalls, request)

	return client.expectation.StubResponse, nil
}

// enqueuePendingOrder adds an order to the queue if it is not already pending.
func (client *asyncClient) enqueuePendingOrder(orderID string) {
	if client.isPendingOrder(orderID) {
		return
	}
	client.pendingOrderID = append(client.pendingOrderID, orderID)
}

// isPendingOrder reports whether an order is currently queued for processing.
func (client *asyncClient) isPendingOrder(orderID string) bool {
	return slices.Contains(client.pendingOrderID, orderID)
}

// removePendingOrder removes an order from the queue after successful processing.
func (client *asyncClient) removePendingOrder(orderID string) {
	for index, pendingOrderID := range client.pendingOrderID {
		if pendingOrderID != orderID {
			continue
		}

		client.pendingOrderID = append(client.pendingOrderID[:index], client.pendingOrderID[index+1:]...)
		return
	}
}
