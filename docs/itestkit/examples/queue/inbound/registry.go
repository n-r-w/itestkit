package queue

import (
	"testing"

	"github.com/n-r-w/itestkit"
)

// newQueueRegistry builds handlers for the event-style queue example.
func newQueueRegistry() itestkit.MapRegistry[*queueClient] {
	return itestkit.NewMapRegistry(map[string]itestkit.Handler[*queueClient]{
		"InitEnvironment":  initEnvironmentHandler{},
		"PublishOrder":     publishOrderHandler{},
		"AwaitConsumption": awaitConsumptionHandler{},
		"VerifyOrder":      verifyOrderHandler{},
	})
}

// queueHarnessFactory creates an isolated case-level harness and implements suite lifecycle.
type queueHarnessFactory struct{}

var (
	_ itestkit.HarnessFactory[*queueClient]                    = (*queueHarnessFactory)(nil)
	_ itestkit.SuiteLifecycle[struct{}]                        = (*queueHarnessFactory)(nil)
	_ itestkit.SuiteCaseHarnessFactory[struct{}, *queueClient] = (*queueHarnessFactory)(nil)
)

// New creates a new in-memory client so cases do not share state.
func (queueHarnessFactory) New(_ *testing.T) *queueClient {
	return newQueueClient()
}

// SetupSuite prepares suite context; no external resources are required in this example.
func (queueHarnessFactory) SetupSuite(_ *testing.T) (struct{}, error) {
	return struct{}{}, nil
}

// TeardownSuite releases suite resources; nothing to release in this example.
func (queueHarnessFactory) TeardownSuite(_ *testing.T, _ struct{}) error {
	return nil
}

// NewCaseHarness creates a case-level harness on top of suite context.
func (factory queueHarnessFactory) NewCaseHarness(t *testing.T, _ struct{}) *queueClient {
	return factory.New(t)
}
