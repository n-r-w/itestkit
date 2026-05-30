package kafkaoutbound

import (
	"testing"

	"github.com/n-r-w/itestkit"
	"github.com/n-r-w/itestkit/queue/itest"
	"github.com/n-r-w/itestkit/queue/kafkaproducer"
	"github.com/stretchr/testify/require"
)

// outboundHarness extends the Kafka helper with domain-specific action behavior.
type outboundHarness struct {
	*kafkaproducer.Harness
}

// outboundHarnessFactory starts Kafka suite once and creates case-level harnesses.
type outboundHarnessFactory struct{}

var (
	_ itestkit.SuiteLifecycle[*kafkaproducer.Suite]                            = (*outboundHarnessFactory)(nil)
	_ itestkit.SuiteCaseHarnessFactory[*kafkaproducer.Suite, *outboundHarness] = (*outboundHarnessFactory)(nil)
	_ itest.OutboundHarness                                                    = (*outboundHarness)(nil)
)

// SetupSuite starts one Kafka container for the whole case suite.
func (outboundHarnessFactory) SetupSuite(t *testing.T) (*kafkaproducer.Suite, error) {
	return kafkaproducer.StartSuite(t.Context())
}

// TeardownSuite stops Kafka container after all cases are completed.
func (outboundHarnessFactory) TeardownSuite(t *testing.T, suite *kafkaproducer.Suite) error {
	if suite == nil {
		return nil
	}
	return suite.Close(t.Context())
}

// NewCaseHarness creates case-level harness for the current case.
func (outboundHarnessFactory) NewCaseHarness(t *testing.T, suite *kafkaproducer.Suite) *outboundHarness {
	harness, err := suite.NewHarness(t.Name())
	require.NoError(t, err)
	return &outboundHarness{Harness: harness}
}
