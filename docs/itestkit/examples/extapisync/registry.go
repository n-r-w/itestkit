package extapisync

import (
	"testing"

	"github.com/n-r-w/itestkit"
)

// newSyncRegistry builds the full handler set for the extapisync example.
func newSyncRegistry() itestkit.MapRegistry[*syncClient] {
	return itestkit.NewMapRegistry(map[string]itestkit.Handler[*syncClient]{
		"SeedData":             seedDataHandler{},
		"PlanExternalCharge":   planExternalChargeHandler{},
		"CreateOrder":          createOrderHandler{},
		"VerifyExternalCharge": verifyExternalChargeHandler{},
	})
}

// syncHarnessFactory creates an isolated harness for each case.
type syncHarnessFactory struct{}

var (
	_ itestkit.HarnessFactory[*syncClient]                    = (*syncHarnessFactory)(nil)
	_ itestkit.SuiteLifecycle[struct{}]                       = (*syncHarnessFactory)(nil)
	_ itestkit.SuiteCaseHarnessFactory[struct{}, *syncClient] = (*syncHarnessFactory)(nil)
)

// New creates a fresh client so cases do not share state.
func (syncHarnessFactory) New(_ *testing.T) *syncClient {
	return newSyncClient()
}

// SetupSuite prepares suite context; no external resources are created in this example.
func (syncHarnessFactory) SetupSuite(_ *testing.T) (struct{}, error) {
	return struct{}{}, nil
}

// TeardownSuite releases suite resources; no cleanup is required in this example.
func (syncHarnessFactory) TeardownSuite(_ *testing.T, _ struct{}) error {
	return nil
}

// NewCaseHarness creates a case-level harness based on suite context.
func (factory syncHarnessFactory) NewCaseHarness(t *testing.T, _ struct{}) *syncClient {
	return factory.New(t)
}
