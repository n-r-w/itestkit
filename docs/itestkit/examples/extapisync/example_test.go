package extapisync

import (
	"embed"
	"testing"

	"github.com/n-r-w/itestkit"
	"github.com/stretchr/testify/require"
)

const casesRootDir = "cases"

//go:embed cases/*.jsonc
var casesFS embed.FS

// TestExternalAPISyncExample shows synchronous outbound-call verification via prepare/action/verify.
func TestExternalAPISyncExample(t *testing.T) {
	t.Parallel()

	codec := syncStatusCodec{}
	cases, err := itestkit.LoadCases(casesFS, casesRootDir, newSyncRegistry(), codec)
	require.NoError(t, err)

	itestkit.RunCases(
		t,
		cases,
		syncHarnessFactory{},
		syncHarnessFactory{},
		syncErrorInspector{},
		codec,
	)
}
