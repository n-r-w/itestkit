//go:build integration

package kafkaproducer

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/n-r-w/itestkit/queue/probe"
	"github.com/stretchr/testify/require"
)

// newIntegrationHarness picks up the suite and returns a case-level harness with cleanup via t.Cleanup.
func newIntegrationHarness(t *testing.T, namespace string) *Harness {
	t.Helper()

	suite, err := StartSuite(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, suite.Close(context.Background()))
	})

	harness, err := suite.NewHarness(namespace)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, harness.CleanupBroker(context.Background()))
	})

	return harness
}

// TestIntegrationHarness_PublishAwaitVerify checks the smoke script publish -> await -> verify on real Kafka.
func TestIntegrationHarness_PublishAwaitVerify(t *testing.T) {
	harness := newIntegrationHarness(t, "integration/publish verify")

	err := harness.PlanOutbound(t.Context(), probe.OutboundExpectation{
		Topic:         "orders",
		Key:           nil,
		Headers:       map[string]string{"x-flow": "itest"},
		HeadersMode:   "",
		ExpectedCount: 1,
		Payload:       nil,
		PayloadSubset: json.RawMessage(`{"event":"created","order_id":"order-100"}`),
		Ordering:      "",
	})
	require.NoError(t, err)

	err = harness.PublishJSON(
		t.Context(),
		"orders",
		nil,
		map[string]string{"x-flow": "itest"},
		map[string]any{"event": "created", "order_id": "order-100", "status": "new"},
	)
	require.NoError(t, err)

	var awaitResult probe.CheckResult
	require.Eventually(t, func() bool {
		attemptCtx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
		defer cancel()

		result, awaitErr := harness.AwaitOutbound(attemptCtx)
		if awaitErr != nil {
			return false
		}

		awaitResult = result
		return true
	}, 20*time.Second, 100*time.Millisecond)
	require.Equal(t, 1, awaitResult.MatchedCount)

	verifyResult, err := harness.VerifyOutbound(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, verifyResult.MatchedCount)
	require.Len(t, verifyResult.ObservedMessages, 1)
	require.Equal(t, "orders", verifyResult.ObservedMessages[0].Topic)
}

// TestIntegrationHarness_AwaitZeroCountFailsOnFirstMatch checks the negative scenario expected_count=0.
func TestIntegrationHarness_AwaitZeroCountFailsOnFirstMatch(t *testing.T) {
	harness := newIntegrationHarness(t, "integration/zero count")

	err := harness.PlanOutbound(t.Context(), probe.OutboundExpectation{
		Topic:         "orders",
		Key:           nil,
		Headers:       map[string]string{"x-flow": "itest"},
		HeadersMode:   "",
		ExpectedCount: 0,
		Payload:       nil,
		PayloadSubset: nil,
		Ordering:      "",
	})
	require.NoError(t, err)

	err = harness.PublishJSON(
		t.Context(),
		"orders",
		nil,
		map[string]string{"x-flow": "itest"},
		map[string]any{"event": "created", "order_id": "order-200"},
	)
	require.NoError(t, err)

	var awaitErr error
	require.Eventually(t, func() bool {
		attemptCtx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
		defer cancel()

		_, awaitErr = harness.AwaitOutbound(attemptCtx)
		if awaitErr == nil {
			return false
		}

		return strings.Contains(awaitErr.Error(), "expected_count=0")
	}, 20*time.Second, 100*time.Millisecond)
	require.Error(t, awaitErr)
	require.Contains(t, awaitErr.Error(), "expected_count=0")
}
