package queue

import "context"

const (
	// consumeReadyAttempt defines the attempt on which the consumer starts processing successfully.
	consumeReadyAttempt = 2
	// queueOrderStatePending marks a published order waiting for consumption.
	queueOrderStatePending = "pending"
	// queueOrderStateProcessed marks an order stored by the consumer.
	queueOrderStateProcessed = "processed"
)

// initEnvironmentRequest describes an in-memory environment setup step.
type initEnvironmentRequest struct{}

// initEnvironmentResponse confirms that the environment is initialized.
type initEnvironmentResponse struct {
	QueueDepth int `json:"queue_depth"`
}

// publishOrderRequest describes a message published to "Kafka".
type publishOrderRequest struct {
	OrderID string `json:"order_id"`
	Amount  int    `json:"amount"`
}

// publishOrderResponse returns publication metadata.
type publishOrderResponse struct {
	Topic  string `json:"topic"`
	Offset int    `json:"offset"`
}

// awaitConsumptionRequest describes waiting for a specific message to be consumed.
type awaitConsumptionRequest struct {
	OrderID string `json:"order_id"`
}

// awaitConsumptionResponse reflects the current consumer process state.
type awaitConsumptionResponse struct {
	State string `json:"state"`
}

// verifyOrderRequest describes an in-memory DB query to verify a stored order.
type verifyOrderRequest struct {
	OrderID string `json:"order_id"`
}

// verifyOrderResponse contains record state compared against assert.response.
type verifyOrderResponse struct {
	OrderID string `json:"order_id"`
	Amount  int    `json:"amount"`
	Stored  bool   `json:"stored"`
}

// brokerMessage models a message in the queue.
type brokerMessage struct {
	OrderID string
	Amount  int
}

// storedOrder models a record stored in the in-memory DB.
type storedOrder struct {
	OrderID string
	Amount  int
}

// queueClient stores in-memory state of the simulated queue, consumer, and DB.
type queueClient struct {
	brokerMessages   []brokerMessage
	storedOrders     map[string]storedOrder
	consumerAttempts map[string]int
	nextOffset       int
}

// newQueueClient creates a clean in-memory state for one case.
func newQueueClient() *queueClient {
	return &queueClient{
		brokerMessages:   make([]brokerMessage, 0),
		storedOrders:     make(map[string]storedOrder),
		consumerAttempts: make(map[string]int),
		nextOffset:       0,
	}
}

// InitEnvironment clears queue and DB for deterministic case startup.
func (client *queueClient) InitEnvironment(
	_ context.Context,
	_ *initEnvironmentRequest,
) (*initEnvironmentResponse, error) {
	client.brokerMessages = client.brokerMessages[:0]
	client.storedOrders = make(map[string]storedOrder)
	client.consumerAttempts = make(map[string]int)
	client.nextOffset = 0

	return &initEnvironmentResponse{QueueDepth: 0}, nil
}

// PublishOrder writes a message to the in-memory queue as a publish step.
func (client *queueClient) PublishOrder(
	_ context.Context,
	request *publishOrderRequest,
) (*publishOrderResponse, error) {
	client.brokerMessages = append(client.brokerMessages, brokerMessage{OrderID: request.OrderID, Amount: request.Amount})
	offset := client.nextOffset
	client.nextOffset++

	return &publishOrderResponse{Topic: "orders", Offset: offset}, nil
}

// AwaitConsumption simulates eventual consistency and moves a message from queue to DB.
func (client *queueClient) AwaitConsumption(
	_ context.Context,
	request *awaitConsumptionRequest,
) (*awaitConsumptionResponse, error) {
	if _, alreadyStored := client.storedOrders[request.OrderID]; alreadyStored {
		return &awaitConsumptionResponse{State: queueOrderStateProcessed}, nil
	}

	message, exists := client.findBrokerMessage(request.OrderID)
	if !exists {
		return &awaitConsumptionResponse{State: "missing"}, queueStatusError{
			code:    queueStatusFailed,
			message: "message is not published",
		}
	}

	client.consumerAttempts[request.OrderID]++
	if client.consumerAttempts[request.OrderID] < consumeReadyAttempt {
		return &awaitConsumptionResponse{State: queueOrderStatePending}, queueStatusError{
			code:    queueStatusFailed,
			message: "consumer has not processed message yet",
		}
	}

	client.storedOrders[request.OrderID] = storedOrder(message)

	return &awaitConsumptionResponse{State: queueOrderStateProcessed}, nil
}

// VerifyOrder reads the in-memory DB and returns a stored record for final assert.
func (client *queueClient) VerifyOrder(
	_ context.Context,
	request *verifyOrderRequest,
) (*verifyOrderResponse, error) {
	order, exists := client.storedOrders[request.OrderID]
	if !exists {
		return &verifyOrderResponse{OrderID: request.OrderID, Amount: 0, Stored: false}, queueStatusError{
			code:    queueStatusFailed,
			message: "order is not stored in database",
		}
	}

	return &verifyOrderResponse{OrderID: order.OrderID, Amount: order.Amount, Stored: true}, nil
}

// findBrokerMessage finds a published message by order ID.
func (client *queueClient) findBrokerMessage(orderID string) (brokerMessage, bool) {
	for _, message := range client.brokerMessages {
		if message.OrderID == orderID {
			return message, true
		}
	}

	return brokerMessage{OrderID: "", Amount: 0}, false
}
