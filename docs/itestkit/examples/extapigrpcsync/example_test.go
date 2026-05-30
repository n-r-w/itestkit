package extapigrpcsync

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

// TestExternalAPIGRPCSyncExample shows end-to-end cases in "inbound API + outbound gRPC mock" style.
//
// The test does not replace the runner and does not do manual checks:
// all expectations and asserts are read from JSONC, as in real itestkit usage.
func TestExternalAPIGRPCSyncExample(t *testing.T) {
	t.Parallel()

	codec := itestkitgrpc.StatusCodec{}
	cases, err := itestkit.LoadCases(casesFS, casesRootDir, newGRPCSyncRegistry(), codec)
	require.NoError(t, err)

	itestkit.RunCases(
		t,
		cases,
		grpcSyncHarnessFactory{},
		grpcSyncHarnessFactory{},
		itestkitgrpc.ErrorInspector{},
		codec,
	)
}
