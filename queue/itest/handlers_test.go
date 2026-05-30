package itest_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/n-r-w/itestkit"
	"github.com/n-r-w/itestkit/queue/itest"
	"github.com/n-r-w/itestkit/queue/probe"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestPlanOutboundHandler_Invoke checks decode+invoke for prepare preset.
func TestPlanOutboundHandler_Invoke(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	handler := itest.PlanOutboundHandler[*itest.MockOutboundHarness]{}
	harness := itest.NewMockOutboundHarness(ctrl)
	ctx := t.Context()

	plannedExpectation := probe.OutboundExpectation{
		Topic:         "",
		Key:           nil,
		Headers:       nil,
		HeadersMode:   "",
		ExpectedCount: 0,
		Payload:       nil,
		PayloadSubset: nil,
		Ordering:      "",
	}
	harness.EXPECT().PlanOutbound(ctx, gomock.Any()).Times(1).DoAndReturn(
		func(_ context.Context, expectation probe.OutboundExpectation) error {
			plannedExpectation = expectation
			return nil
		},
	)

	requestRaw := json.RawMessage(`{
		"topic":"orders",
		"expected_count":1,
		"payload":{"order_id":"order-100"}
	}`)

	request, err := handler.DecodeRequest(requestRaw)
	require.NoError(t, err)

	response, err := handler.Invoke(ctx, harness, request)
	require.NoError(t, err)
	require.Equal(t, "orders", plannedExpectation.Topic)
	require.Equal(t, 1, plannedExpectation.ExpectedCount)
	require.JSONEq(t, `{"order_id":"order-100"}`, string(plannedExpectation.Payload))

	typedResponse, ok := response.(*itest.PlanOutboundResponse)
	require.True(t, ok)
	require.True(t, typedResponse.Planned)
	require.Equal(t, 1, typedResponse.ExpectedCount)
}

// TestAwaitOutboundHandler_Invoke checks the await check call via harness.
func TestAwaitOutboundHandler_Invoke(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	expectedResult := probe.CheckResult{MatchedCount: 2, ObservedMessages: nil, LastMismatchReason: ""}
	harness := itest.NewMockOutboundHarness(ctrl)
	handler := itest.AwaitOutboundHandler[*itest.MockOutboundHarness]{}
	ctx := t.Context()

	harness.EXPECT().AwaitOutbound(ctx).Times(1).Return(expectedResult, nil)

	request, err := handler.DecodeRequest(json.RawMessage(`{}`))
	require.NoError(t, err)

	response, err := handler.Invoke(ctx, harness, request)
	require.NoError(t, err)
	require.Equal(t, expectedResult, response)
}

// TestVerifyOutboundHandler_Invoke verifies the verify call via harness.
func TestVerifyOutboundHandler_Invoke(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	expectedResult := probe.CheckResult{MatchedCount: 1, ObservedMessages: nil, LastMismatchReason: ""}
	harness := itest.NewMockOutboundHarness(ctrl)
	handler := itest.VerifyOutboundHandler[*itest.MockOutboundHarness]{}
	ctx := t.Context()

	harness.EXPECT().VerifyOutbound(ctx).Times(1).Return(expectedResult, nil)

	request, err := handler.DecodeRequest(json.RawMessage(`{}`))
	require.NoError(t, err)

	response, err := handler.Invoke(ctx, harness, request)
	require.NoError(t, err)
	require.Equal(t, expectedResult, response)
}

// TestCleanupBrokerHandler_Invoke checks the cleanup check call via harness.
func TestCleanupBrokerHandler_Invoke(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	harness := itest.NewMockOutboundHarness(ctrl)
	handler := itest.CleanupBrokerHandler[*itest.MockOutboundHarness]{}
	ctx := t.Context()

	harness.EXPECT().CleanupBroker(ctx).Times(1).Return(nil)

	request, err := handler.DecodeRequest(json.RawMessage(`{}`))
	require.NoError(t, err)

	response, err := handler.Invoke(ctx, harness, request)
	require.NoError(t, err)

	typedResponse, ok := response.(*itest.CleanupBrokerResponse)
	require.True(t, ok)
	require.True(t, typedResponse.Cleaned)
}

// TestNewRegistry_IncludesPresetAndCustom checks that the registry contains preset and custom handlers.
func TestNewRegistry_IncludesPresetAndCustom(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	custom := itestkit.NewMockHandler[*itest.MockOutboundHarness](ctrl)

	registry := itest.NewRegistry[*itest.MockOutboundHarness](map[string]itestkit.Handler[*itest.MockOutboundHarness]{
		"CustomAction": custom,
	})

	supported := registry.Supported()
	require.Contains(t, supported, itest.PlanOutboundHandlerName)
	require.Contains(t, supported, itest.AwaitOutboundHandlerName)
	require.Contains(t, supported, itest.VerifyOutboundHandlerName)
	require.Contains(t, supported, itest.CleanupBrokerHandlerName)
	require.Contains(t, supported, "CustomAction")
}
