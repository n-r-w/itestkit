package extapiasync

import (
	"embed"
	"testing"

	"github.com/n-r-w/itestkit"
	"github.com/stretchr/testify/require"
)

const casesRootDir = "cases"

//go:embed cases/*.jsonc
var casesFS embed.FS

// TestExternalAPIAsyncExample shows an action -> await -> verify async pipeline for an external API.
func TestExternalAPIAsyncExample(t *testing.T) {
	t.Parallel()

	codec := asyncStatusCodec{}
	cases, err := itestkit.LoadCases(casesFS, casesRootDir, newAsyncRegistry(), codec)
	require.NoError(t, err)

	itestkit.RunCases(
		t,
		cases,
		asyncHarnessFactory{},
		asyncHarnessFactory{},
		asyncErrorInspector{},
		codec,
	)
}
