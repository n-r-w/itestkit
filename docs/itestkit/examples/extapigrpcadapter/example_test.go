package extapigrpcadapter

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

// TestExternalAPIAdapterExample demonstrates adapter-style integration via bufconn.NewServer.
func TestExternalAPIAdapterExample(t *testing.T) {
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
