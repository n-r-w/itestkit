package httpmockexample

import (
	"testing"

	itestkithttpmock "github.com/n-r-w/itestkit/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestZeroOutboundCallsExample shows how an empty strict plan verifies that no request was observed.
func TestZeroOutboundCallsExample(t *testing.T) {
	t.Parallel()

	server := itestkithttpmock.NewServer(t)
	require.NoError(t, server.Plan(t.Context(), itestkithttpmock.Plan{
		Calls:    []itestkithttpmock.CallExpectation{},
		Ordering: itestkithttpmock.OrderingStrict,
	}))

	result, err := server.Verify(t.Context())

	require.NoError(t, err)
	assert.Zero(t, result.MatchedCount)
}
