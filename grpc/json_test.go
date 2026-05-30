package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestDecodeProtoJSON_NilMessage(t *testing.T) {
	t.Parallel()

	err := DecodeProtoJSON(json.RawMessage(`{}`), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil message")
}

func TestErrorInspector_FromError_ContextErrors(t *testing.T) {
	t.Parallel()

	inspector := ErrorInspector{}

	code, msg, ok := inspector.FromError(context.Canceled)
	require.True(t, ok)
	require.Equal(t, codes.Canceled, code)
	require.Equal(t, context.Canceled.Error(), msg)

	code, msg, ok = inspector.FromError(fmt.Errorf("wrapped: %w", context.DeadlineExceeded))
	require.True(t, ok)
	require.Equal(t, codes.DeadlineExceeded, code)
	require.Equal(t, "wrapped: "+context.DeadlineExceeded.Error(), msg)
}

func TestErrorInspector_FromError_StatusError(t *testing.T) {
	t.Parallel()

	inspector := ErrorInspector{}

	err := status.Error(codes.NotFound, "missing")
	code, msg, ok := inspector.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.NotFound, code)
	require.Equal(t, "missing", msg)
}

func TestNormalizeProtoMessage_EmptyObjectIsStable(t *testing.T) {
	t.Parallel()

	normalized, err := NormalizeProtoMessage(&emptypb.Empty{})
	require.NoError(t, err)
	require.Equal(t, map[string]any{}, normalized)
}
