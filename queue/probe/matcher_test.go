package probe

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNormalizeExpectation_DefaultsAndValidation checks default values ​​and basic contract validation.
func TestNormalizeExpectation_DefaultsAndValidation(t *testing.T) {
	t.Parallel()

	expectation := OutboundExpectation{
		Topic:         "  orders  ",
		Key:           nil,
		Headers:       nil,
		HeadersMode:   "",
		ExpectedCount: 1,
		Payload:       rawJSON(t, `{"order_id":"100"}`),
		PayloadSubset: nil,
		Ordering:      "",
	}

	normalized, err := NormalizeExpectation(expectation)
	require.NoError(t, err)
	require.Equal(t, "orders", normalized.Topic)
	require.Equal(t, HeadersModeExact, normalized.HeadersMode)
	require.Equal(t, OrderingAny, normalized.Ordering)
}

// TestNormalizeExpectation_RejectsInvalidPayloadRules checks the XOR validation of payload and payload_subset.
func TestNormalizeExpectation_RejectsInvalidPayloadRules(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		expectation OutboundExpectation
		errorText   string
	}{
		{
			name: "expected_count_zero_with_payload",
			expectation: OutboundExpectation{
				Topic:         "orders",
				Key:           nil,
				Headers:       nil,
				HeadersMode:   "",
				ExpectedCount: 0,
				Payload:       rawJSON(t, `{"order_id":"100"}`),
				PayloadSubset: nil,
				Ordering:      "",
			},
			errorText: "forbidden when expected_count=0",
		},
		{
			name: "expected_count_positive_without_payload",
			expectation: OutboundExpectation{
				Topic:         "orders",
				Key:           nil,
				Headers:       nil,
				HeadersMode:   "",
				ExpectedCount: 1,
				Payload:       nil,
				PayloadSubset: nil,
				Ordering:      "",
			},
			errorText: "exactly one of fields payload and payload_subset",
		},
		{
			name: "expected_count_positive_with_both_payloads",
			expectation: OutboundExpectation{
				Topic:         "orders",
				Key:           nil,
				Headers:       nil,
				HeadersMode:   "",
				ExpectedCount: 1,
				Payload:       rawJSON(t, `{"order_id":"100"}`),
				PayloadSubset: rawJSON(t, `{"order_id":"100"}`),
				Ordering:      "",
			},
			errorText: "exactly one of fields payload and payload_subset",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := NormalizeExpectation(testCase.expectation)
			require.Error(t, err)
			require.Contains(t, err.Error(), testCase.errorText)
		})
	}
}

// TestEvaluateVerify_HeadersMismatch tests the header mismatch diagnostic.
func TestEvaluateVerify_HeadersMismatch(t *testing.T) {
	t.Parallel()

	key := "key-100"
	expectation, err := NormalizeExpectation(OutboundExpectation{
		Topic:         "orders",
		Key:           &key,
		Headers:       map[string]string{"x-flow": "itest"},
		HeadersMode:   "",
		ExpectedCount: 1,
		Payload:       rawJSON(t, `{"order_id":"100"}`),
		PayloadSubset: nil,
		Ordering:      "",
	})
	require.NoError(t, err)

	result, verifyErr := EvaluateVerify(expectation, []Message{
		newMessage(t, "orders", key, map[string]string{"x-flow": "wrong"}, `{"order_id":"100"}`, 10),
	})
	require.Error(t, verifyErr)
	require.Equal(t, 0, result.MatchedCount)
	require.Contains(t, result.LastMismatchReason, "headers mismatch")
}

// TestEvaluateVerify_OrderingStrict checks the strict ordering mode and the absence of unnecessary messages.
func TestEvaluateVerify_OrderingStrict(t *testing.T) {
	t.Parallel()

	expectation, err := NormalizeExpectation(OutboundExpectation{
		Topic:         "orders",
		Key:           nil,
		Headers:       nil,
		HeadersMode:   "",
		ExpectedCount: 2,
		Payload:       nil,
		PayloadSubset: rawJSON(t, `{"type":"created"}`),
		Ordering:      OrderingStrict,
	})
	require.NoError(t, err)

	_, verifyErr := EvaluateVerify(expectation, []Message{
		newMessage(t, "orders", "", nil, `{"type":"created","id":"1"}`, 1),
		newMessage(t, "orders", "", nil, `{"type":"ignored","id":"x"}`, 2),
	})
	require.Error(t, verifyErr)
	require.Contains(t, verifyErr.Error(), "strict ordering mismatch")
}

// TestEvaluateVerify_OrderingAnyAndCount checks the strict count for ordering=any.
func TestEvaluateVerify_OrderingAnyAndCount(t *testing.T) {
	t.Parallel()

	expectation, err := NormalizeExpectation(OutboundExpectation{
		Topic:         "orders",
		Key:           nil,
		Headers:       nil,
		HeadersMode:   "",
		ExpectedCount: 2,
		Payload:       nil,
		PayloadSubset: rawJSON(t, `{"type":"created"}`),
		Ordering:      OrderingAny,
	})
	require.NoError(t, err)

	result, verifyErr := EvaluateVerify(expectation, []Message{
		newMessage(t, "orders", "", nil, `{"type":"created","id":"1"}`, 1),
		newMessage(t, "orders", "", nil, `{"type":"created","id":"2"}`, 2),
		newMessage(t, "orders", "", nil, `{"type":"created","id":"3"}`, 3),
	})
	require.Error(t, verifyErr)
	require.Equal(t, 3, result.MatchedCount)
	require.Contains(t, verifyErr.Error(), "expected matched_count=2")
}

// TestEvaluateVerify_PayloadSubsetNestedSemantics locks the JSON subset rules used by queue probes.
func TestEvaluateVerify_PayloadSubsetNestedSemantics(t *testing.T) {
	t.Parallel()

	expectation, err := NormalizeExpectation(OutboundExpectation{
		Topic:         "orders",
		Key:           nil,
		Headers:       nil,
		HeadersMode:   "",
		ExpectedCount: 1,
		Payload:       nil,
		PayloadSubset: rawJSON(t, `{"meta":{"source":"api"},"items":[{"sku":"sku-1"}]}`),
		Ordering:      OrderingAny,
	})
	require.NoError(t, err)

	result, verifyErr := EvaluateVerify(expectation, []Message{
		newMessage(
			t,
			"orders",
			"",
			nil,
			`{"meta":{"source":"api","trace":"trace-1"},"items":[{"sku":"sku-1","qty":2},{"sku":"sku-2"}]}`,
			1,
		),
	})
	require.NoError(t, verifyErr)
	require.Equal(t, 1, result.MatchedCount)
}

// TestEvaluateAwait_ZeroExpectedCountWindowSemantics checks the expected_count=0 await semantics.
func TestEvaluateAwait_ZeroExpectedCountWindowSemantics(t *testing.T) {
	t.Parallel()

	expectation, err := NormalizeExpectation(OutboundExpectation{
		Topic:         "orders",
		Key:           nil,
		ExpectedCount: 0,
		Headers:       map[string]string{"x-flow": "itest"},
		HeadersMode:   "",
		Payload:       nil,
		PayloadSubset: nil,
		Ordering:      "",
	})
	require.NoError(t, err)

	_, awaitErr := EvaluateAwait(expectation, nil, false)
	require.Error(t, awaitErr)
	require.Contains(t, awaitErr.Error(), "retry window is still open")

	_, awaitErr = EvaluateAwait(expectation, []Message{
		newMessage(t, "orders", "", map[string]string{"x-flow": "itest"}, `{"id":"100"}`, 1),
	}, true)
	require.Error(t, awaitErr)
	require.Contains(t, awaitErr.Error(), "expected_count=0")

	result, awaitErr := EvaluateAwait(expectation, nil, true)
	require.NoError(t, awaitErr)
	assert.Equal(t, 0, result.MatchedCount)
}

// TestEvaluateAwait_PositiveExpectedCount checks for the successful completion of await when count is reached.
func TestEvaluateAwait_PositiveExpectedCount(t *testing.T) {
	t.Parallel()

	expectation, err := NormalizeExpectation(OutboundExpectation{
		Topic:         "orders",
		Key:           nil,
		Headers:       nil,
		HeadersMode:   "",
		ExpectedCount: 2,
		Payload:       nil,
		PayloadSubset: rawJSON(t, `{"type":"created"}`),
		Ordering:      "",
	})
	require.NoError(t, err)

	result, awaitErr := EvaluateAwait(expectation, []Message{
		newMessage(t, "orders", "", nil, `{"type":"created","id":"1"}`, 1),
		newMessage(t, "orders", "", nil, `{"type":"created","id":"2"}`, 2),
	}, false)
	require.NoError(t, awaitErr)
	require.Equal(t, 2, result.MatchedCount)
}

// rawJSON prepares json.RawMessage for test expectations.
func rawJSON(t *testing.T, value string) json.RawMessage {
	t.Helper()

	return json.RawMessage(value)
}

// newMessage creates an observable message for matcher tests.
func newMessage(
	t *testing.T,
	topic string,
	key string,
	headers map[string]string,
	payload string,
	offset int64,
) Message {
	t.Helper()

	var keyPtr *string
	if key != "" {
		keyCopy := key
		keyPtr = &keyCopy
	}

	payloadRaw := rawJSON(t, payload)

	return Message{
		Topic:   topic,
		Key:     keyPtr,
		Headers: headers,
		Payload: payloadRaw,
		Offset:  offset,
	}
}
