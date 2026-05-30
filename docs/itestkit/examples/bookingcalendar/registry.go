package bookingcalendar

import (
	"testing"

	"github.com/n-r-w/itestkit"
	"github.com/n-r-w/itestkit/testcalendar"
)

// newRegistry builds the handler set used by the calendar example.
func newRegistry() itestkit.MapRegistry[*bookingClient] {
	return itestkit.NewMapRegistry(map[string]itestkit.Handler[*bookingClient]{
		"SeedData":            seedDataHandler{},
		"PlanExternalQuote":   planExternalQuoteHandler{},
		"CreateBookingQuote":  createBookingQuoteHandler{},
		"VerifyExternalQuote": verifyExternalQuoteHandler{},
	})
}

// harnessFactory creates an isolated client with an injectable now-provider.
type harnessFactory struct{}

var (
	_ itestkit.SuiteLifecycle[struct{}]                          = (*harnessFactory)(nil)
	_ itestkit.SuiteCaseHarnessFactory[struct{}, *bookingClient] = (*harnessFactory)(nil)
)

// SetupSuite prepares suite context; this example needs no shared resources.
func (harnessFactory) SetupSuite(_ *testing.T) (struct{}, error) {
	return struct{}{}, nil
}

// TeardownSuite releases suite resources; this example has nothing to clean up.
func (harnessFactory) TeardownSuite(_ *testing.T, _ struct{}) error {
	return nil
}

// NewCaseHarness creates a case-level harness from the suite context.
func (harnessFactory) NewCaseHarness(_ *testing.T, _ struct{}) *bookingClient {
	return newBookingClient(testcalendar.FixedNow)
}
