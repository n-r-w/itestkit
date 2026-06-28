package httpserverexample

import (
	"embed"
	"testing"

	"github.com/n-r-w/itestkit"
	"github.com/stretchr/testify/require"
)

const casesRootDir = "cases"

//go:embed cases/*.jsonc
var casesFS embed.FS

// TestHTTPServerExample shows JSONC-driven calls to an in-process HTTP handler.
func TestHTTPServerExample(t *testing.T) {
	t.Parallel()

	codec := statusCodec{}
	cases, err := itestkit.LoadCases(casesFS, casesRootDir, newRegistry(), codec)
	require.NoError(t, err)

	itestkit.RunCases(
		t,
		cases,
		harnessFactory{},
		harnessFactory{},
		errorInspector{},
		codec,
	)
}
