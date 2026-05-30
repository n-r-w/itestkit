package grpc

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// testClient describes the test client for the handler spec.
type testClient struct{}

// TestNewHandlerSpec_TypeSafety checks the type control for request/response.
func TestNewHandlerSpec_TypeSafety(t *testing.T) {
	t.Parallel()

	spec := NewHandlerSpec(
		"Test",
		func() *emptypb.Empty { return &emptypb.Empty{} },
		func() *emptypb.Empty { return &emptypb.Empty{} },
		func(_ context.Context, _ testClient, _ *emptypb.Empty) (*emptypb.Empty, error) {
			return &emptypb.Empty{}, nil
		},
		func(_ *emptypb.Empty) (any, error) {
			return "ok", nil
		},
	)

	_, err := spec.Invoke(t.Context(), testClient{}, &wrapperspb.StringValue{Value: ""})
	require.Error(t, err)

	_, err = spec.NormalizeResponse(&wrapperspb.StringValue{Value: ""})
	require.Error(t, err)
}

// TestRegistry_Supported_Sorted checks the sorting of the supported list.
func TestRegistry_Supported_Sorted(t *testing.T) {
	t.Parallel()

	specA := NewHandlerSpec(
		"A",
		func() *emptypb.Empty { return &emptypb.Empty{} },
		func() *emptypb.Empty { return &emptypb.Empty{} },
		func(_ context.Context, _ testClient, _ *emptypb.Empty) (*emptypb.Empty, error) {
			return &emptypb.Empty{}, nil
		},
		nil,
	)
	specB := NewHandlerSpec(
		"B",
		func() *emptypb.Empty { return &emptypb.Empty{} },
		func() *emptypb.Empty { return &emptypb.Empty{} },
		func(_ context.Context, _ testClient, _ *emptypb.Empty) (*emptypb.Empty, error) {
			return &emptypb.Empty{}, nil
		},
		nil,
	)

	registry := NewRegistry(map[string]HandlerSpec[testClient]{
		"B": specB,
		"A": specA,
	})

	require.Equal(t, []string{"A", "B"}, registry.Supported())
}

// TestNewHandlerSpec_NormalizeResponse_ProtoDeepEqualHazard shows why
// NormalizeResponse should return a DeepEqual-stable representation for proto messages,
// when normalize == nil.
func TestNewHandlerSpec_NormalizeResponse_ProtoDeepEqualHazard(t *testing.T) {
	t.Parallel()

	spec := NewHandlerSpec(
		"Test",
		func() *emptypb.Empty { return &emptypb.Empty{} },
		func() *wrapperspb.BytesValue { return wrapperspb.Bytes(nil) },
		func(_ context.Context, _ testClient, _ *emptypb.Empty) (*wrapperspb.BytesValue, error) {
			return wrapperspb.Bytes(nil), nil
		},
		nil,
	)

	registry := NewRegistry(map[string]HandlerSpec[testClient]{
		"Test": spec,
	})

	handler, err := registry.Resolve("Test")
	require.NoError(t, err)

	expected := wrapperspb.Bytes(nil)
	actual := wrapperspb.Bytes([]byte{})

	require.True(t, proto.Equal(expected, actual))
	require.False(t, reflect.DeepEqual(expected, actual))

	expectedNormalized, err := handler.NormalizeResponse(expected)
	require.NoError(t, err)
	actualNormalized, err := handler.NormalizeResponse(actual)
	require.NoError(t, err)
	_, normalizedIsProto := expectedNormalized.(proto.Message)
	require.False(t, normalizedIsProto, "expected normalized response to not be a proto.Message")

	require.Truef(
		t,
		reflect.DeepEqual(expectedNormalized, actualNormalized),
		"proto.Equal is true, but DeepEqual differs for semantically equal proto messages: expected=%T actual=%T",
		expectedNormalized,
		actualNormalized,
	)
}

// TestRegistry_Resolve_UsesCustomDecodeHooks checks that the spec can override the decode path.
func TestRegistry_Resolve_UsesCustomDecodeHooks(t *testing.T) {
	t.Parallel()

	requestDecoded := false
	expectedDecoded := false

	spec := NewHandlerSpec(
		"Test",
		func() *wrapperspb.StringValue { return &wrapperspb.StringValue{Value: ""} },
		func() *wrapperspb.StringValue { return &wrapperspb.StringValue{Value: ""} },
		func(_ context.Context, _ testClient, request *wrapperspb.StringValue) (*wrapperspb.StringValue, error) {
			return request, nil
		},
		nil,
	)
	spec.DecodeRequestJSON = func(_ json.RawMessage, request proto.Message) error {
		requestDecoded = true
		typedRequest, ok := request.(*wrapperspb.StringValue)
		require.True(t, ok)
		typedRequest.Value = "request-from-hook"
		return nil
	}
	spec.DecodeExpectedResponseJSON = func(_ json.RawMessage, response proto.Message) error {
		expectedDecoded = true
		typedResponse, ok := response.(*wrapperspb.StringValue)
		require.True(t, ok)
		typedResponse.Value = "expected-from-hook"
		return nil
	}

	registry := NewRegistry(map[string]HandlerSpec[testClient]{
		"Test": spec,
	})

	handler, err := registry.Resolve("Test")
	require.NoError(t, err)

	request, err := handler.DecodeRequest(json.RawMessage(`{"ignored":true}`))
	require.NoError(t, err)
	require.True(t, requestDecoded)
	require.Equal(t, "request-from-hook", request.(*wrapperspb.StringValue).GetValue())

	expected, err := handler.DecodeExpectedResponse(json.RawMessage(`{"ignored":true}`))
	require.NoError(t, err)
	require.True(t, expectedDecoded)
	require.Equal(t, "expected-from-hook", expected.(*wrapperspb.StringValue).GetValue())
}
