package extapiasync

import (
	"testing"

	"github.com/n-r-w/itestkit"
)

// newAsyncRegistry builds the full handler set for the extapiasync example.
func newAsyncRegistry() itestkit.MapRegistry[*asyncClient] {
	return itestkit.NewMapRegistry(map[string]itestkit.Handler[*asyncClient]{
		"SeedData":             seedDataHandler{},
		"PlanExternalCharge":   planExternalChargeHandler{},
		"SubmitOrder":          submitOrderHandler{},
		"AwaitExternalSync":    awaitExternalSyncHandler{},
		"VerifyExternalCharge": verifyExternalChargeHandler{},
		"GetOrder":             getOrderHandler{},
	})
}

// asyncHarnessFactory creates an isolated harness and implements suite lifecycle.
type asyncHarnessFactory struct{}

var (
	_ itestkit.HarnessFactory[*asyncClient]                    = (*asyncHarnessFactory)(nil)
	_ itestkit.SuiteLifecycle[struct{}]                        = (*asyncHarnessFactory)(nil)
	_ itestkit.SuiteCaseHarnessFactory[struct{}, *asyncClient] = (*asyncHarnessFactory)(nil)
)

// New creates a new in-memory client for a case.
func (asyncHarnessFactory) New(_ *testing.T) *asyncClient {
	return newAsyncClient()
}

// SetupSuite prepares suite context; no resources are created in this example.
func (asyncHarnessFactory) SetupSuite(_ *testing.T) (struct{}, error) {
	return struct{}{}, nil
}

// TeardownSuite releases suite resources; nothing to release in this example.
func (asyncHarnessFactory) TeardownSuite(_ *testing.T, _ struct{}) error {
	return nil
}

// NewCaseHarness creates a case-level harness on top of suite context.
func (factory asyncHarnessFactory) NewCaseHarness(t *testing.T, _ struct{}) *asyncClient {
	return factory.New(t)
}
