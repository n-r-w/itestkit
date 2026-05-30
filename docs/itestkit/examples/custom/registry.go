package custom

import (
	"testing"

	"github.com/n-r-w/itestkit"
)

// newExampleRegistry builds the minimal handler set for cases.
func newExampleRegistry() itestkit.MapRegistry[*echoClient] {
	return itestkit.NewMapRegistry(map[string]itestkit.Handler[*echoClient]{
		"SetPrefix":      setPrefixHandler{},
		"Echo":           echoHandler{},
		"PublishEvent":   publishEventHandler{},
		"AwaitProcessed": awaitProcessedHandler{},
		"VerifyState":    verifyStateHandler{},
	})
}

// exampleHarnessFactory creates a fresh client for each case.
type exampleHarnessFactory struct{}

var (
	_ itestkit.HarnessFactory[*echoClient]                    = (*exampleHarnessFactory)(nil)
	_ itestkit.SuiteLifecycle[struct{}]                       = (*exampleHarnessFactory)(nil)
	_ itestkit.SuiteCaseHarnessFactory[struct{}, *echoClient] = (*exampleHarnessFactory)(nil)
)

// New returns a new client so steps do not affect each other.
func (exampleHarnessFactory) New(_ *testing.T) *echoClient {
	return &echoClient{
		prefix:       "",
		publishedID:  "",
		processed:    false,
		awaitAttempt: 0,
	}
}

// SetupSuite prepares suite context for a case set.
func (exampleHarnessFactory) SetupSuite(_ *testing.T) (struct{}, error) {
	return struct{}{}, nil
}

// TeardownSuite releases suite resources (none in this example).
func (exampleHarnessFactory) TeardownSuite(_ *testing.T, _ struct{}) error {
	return nil
}

// NewCaseHarness creates an isolated harness for a single case.
func (factory exampleHarnessFactory) NewCaseHarness(t *testing.T, _ struct{}) *echoClient {
	return factory.New(t)
}
