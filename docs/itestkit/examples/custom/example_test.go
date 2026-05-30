package custom

import (
	"embed"
	"testing"

	"github.com/n-r-w/itestkit"
	"github.com/stretchr/testify/require"
)

const casesRootDir = "cases"

//go:embed cases/*.jsonc
var casesFS embed.FS

// TestItestkitExample shows a minimal itestkit run with one case.
func TestItestkitExample(t *testing.T) {
	t.Parallel()

	codec := simpleStatusCodec{}
	cases, err := itestkit.LoadCases(casesFS, casesRootDir, newExampleRegistry(), codec)
	require.NoError(t, err)

	itestkit.RunCases(
		t,
		cases,
		exampleHarnessFactory{},
		exampleHarnessFactory{},
		simpleErrorInspector{},
		codec,
	)
}
