package queue

import (
	"embed"
	"testing"

	"github.com/n-r-w/itestkit"
	"github.com/stretchr/testify/require"
)

const casesRootDir = "cases"

//go:embed cases/*.jsonc
var casesFS embed.FS

// TestQueueExample shows a publish -> await -> verify pipeline with broker and DB simulation.
func TestQueueExample(t *testing.T) {
	t.Parallel()

	codec := queueStatusCodec{}
	cases, err := itestkit.LoadCases(casesFS, casesRootDir, newQueueRegistry(), codec)
	require.NoError(t, err)

	itestkit.RunCases(
		t,
		cases,
		queueHarnessFactory{},
		queueHarnessFactory{},
		queueErrorInspector{},
		codec,
	)
}
