package httpmockexample

import (
	"embed"
	"testing"

	"github.com/n-r-w/itestkit"
	"github.com/stretchr/testify/require"
)

const casesRootDir = "cases"

//go:embed cases/*.jsonc
var casesFS embed.FS

// TestHTTPMockExample shows outbound HTTP planning, stubbing, and verification from JSONC.
func TestHTTPMockExample(t *testing.T) {
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
