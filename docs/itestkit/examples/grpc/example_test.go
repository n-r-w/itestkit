package grpc

import (
	"embed"
	"testing"

	"github.com/n-r-w/itestkit"
	testkitgrpc "github.com/n-r-w/itestkit/grpc"
	"github.com/stretchr/testify/require"
)

const casesRootDir = "cases"

//go:embed cases/*.jsonc
var casesFS embed.FS

// TestGRPCExample shows a minimal itestkit/grpc run with one case.
func TestGRPCExample(t *testing.T) {
	t.Parallel()

	codec := testkitgrpc.StatusCodec{}
	cases, err := itestkit.LoadCases(casesFS, casesRootDir, newHandlerRegistry(), codec)
	require.NoError(t, err)

	itestkit.RunCases(
		t,
		cases,
		grpcHarnessFactory{},
		grpcHarnessFactory{},
		testkitgrpc.ErrorInspector{},
		codec,
		itestkit.WithContinueOnFailure(),
	)
}
