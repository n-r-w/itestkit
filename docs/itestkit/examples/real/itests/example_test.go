package itests

import (
	"embed"
	"testing"

	"github.com/n-r-w/itestkit"
	itestkitgrpc "github.com/n-r-w/itestkit/grpc"
	"github.com/stretchr/testify/require"
)

const casesRootDir = "cases"

//go:embed cases/*.jsonc
var casesFS embed.FS

// TestRealServiceIntegration runs integration JSONC cases for a realistic service structure.
func TestRealServiceIntegration(t *testing.T) {
	t.Parallel()

	codec := itestkitgrpc.StatusCodec{}
	cases, err := itestkit.LoadCases(casesFS, casesRootDir, newHandlerRegistry(), codec)
	require.NoError(t, err)

	itestkit.RunCases(
		t,
		cases,
		harnessFactory{},
		harnessFactory{},
		itestkitgrpc.ErrorInspector{},
		codec,
	)
}
