package grpc

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestStatusCodec_Parse_Unsupported checks for an error when the code is unsupported.
func TestStatusCodec_Parse_Unsupported(t *testing.T) {
	t.Parallel()

	codec := StatusCodec{}
	for _, raw := range []string{"not_a_code", "CANCELED"} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()

			_, err := codec.Parse(raw)
			require.Error(t, err)
			require.Contains(t, err.Error(), "unsupported code")
		})
	}
}

// TestErrorInspector_FromError tests status extraction from a gRPC error.
func TestErrorInspector_FromError(t *testing.T) {
	t.Parallel()

	inspector := ErrorInspector{}
	expectedErr := status.Error(codes.NotFound, "missing")

	code, message, ok := inspector.FromError(expectedErr)
	require.True(t, ok)
	require.Equal(t, codes.NotFound, code)
	require.Equal(t, "missing", message)

	code, message, ok = inspector.FromError(errors.New("boom"))
	require.False(t, ok)
	require.Equal(t, codes.Unknown, code)
	require.Empty(t, message)
}
