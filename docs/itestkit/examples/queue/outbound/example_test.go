package kafkaoutbound

import (
	"embed"
	"testing"

	"github.com/n-r-w/itestkit"
	"github.com/stretchr/testify/require"
)

const casesRootDir = "cases"

//go:embed cases/*.jsonc
var casesFS embed.FS

// TestKafkaOutboundExample demonstrates outbound prepare/action/await/verify/cleanup flow.
func TestKafkaOutboundExample(t *testing.T) {
	t.Parallel()

	codec := outboundStatusCodec{}
	cases, err := itestkit.LoadCases(casesFS, casesRootDir, newRegistry(), codec)
	require.NoError(t, err)

	itests := outboundHarnessFactory{}
	itestkit.RunCases(
		t,
		cases,
		itests,
		itests,
		outboundErrorInspector{},
		codec,
	)
}
