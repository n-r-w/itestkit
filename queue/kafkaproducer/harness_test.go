package kafkaproducer

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/n-r-w/itestkit/queue/probe"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// newTestHarness creates a harness for tests with explicit initialization of all fields.
func newTestHarness(
	brokers []string,
	caseNamespace string,
	writer messageWriter,
	reader messageReader,
	ensureTopicFn func(context.Context, string) error,
	newReaderFn func(string) (messageReader, error),
) *Harness {
	return &Harness{
		brokers:       brokers,
		caseNamespace: caseNamespace,
		writer:        writer,
		reader:        reader,
		ensureTopicFn: ensureTopicFn,
		newReaderFn:   newReaderFn,
		mu:            sync.Mutex{},
		plan:          nil,
		observed:      nil,
	}
}

// TestHarness_ResolveTopic checks the namespace isolation of topic names.
func TestHarness_ResolveTopic(t *testing.T) {
	t.Parallel()

	harness := newTestHarness(nil, normalizeNamespace("case-a"), nil, nil, nil, nil)
	require.Equal(t, "case-a-orders", harness.ResolveTopic("orders"))

	harnessWithoutNamespace := newTestHarness(nil, "", nil, nil, nil, nil)
	require.Equal(t, "orders", harnessWithoutNamespace.ResolveTopic("orders"))
}

// TestNormalizeNamespace checks case namespace normalization for a safe topic-prefix.
func TestNormalizeNamespace(t *testing.T) {
	t.Parallel()

	require.Empty(t, normalizeNamespace("   "))
	require.Equal(t, "case-a-orders", normalizeNamespace("Case A / Orders"))
	require.Equal(t, "suite__case-1", normalizeNamespace("suite__case#1"))
}

// TestHarness_PublishRawAndVerifyFlow checks the publication and verify flow via gomock reader/writer.
func TestHarness_PublishRawAndVerifyFlow(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	writer := NewMockmessageWriter(ctrl)
	reader := NewMockmessageReader(ctrl)

	written := make([]harnessMessage, 0, 1)
	writer.EXPECT().WriteMessages(gomock.Any(), gomock.Any()).Times(1).DoAndReturn(
		func(_ context.Context, messages ...harnessMessage) error {
			written = append(written, messages...)
			return nil
		},
	)

	readerMessages := []harnessMessage{
		{
			Topic:   "case-a-orders",
			Offset:  1,
			Key:     nil,
			Value:   []byte(`{"event":"created","order_id":"100"}`),
			Headers: nil,
		},
	}
	reader.EXPECT().FetchMessage(gomock.Any()).AnyTimes().DoAndReturn(
		func(_ context.Context) (harnessMessage, error) {
			if len(readerMessages) == 0 {
				return deadlineExceededMessage(), context.DeadlineExceeded
			}

			message := readerMessages[0]
			readerMessages = readerMessages[1:]
			return message, nil
		},
	)

	harness := newTestHarness(
		[]string{"127.0.0.1:9092"},
		"case-a",
		writer,
		nil,
		func(_ context.Context, _ string) error { return nil },
		func(_ string) (messageReader, error) { return reader, nil },
	)

	key := "k-100"
	err := harness.PublishRaw(t.Context(), PublishMessage{
		Topic:   "orders",
		Key:     &key,
		Headers: map[string]string{"x-flow": "itest"},
		Payload: []byte(`{"event":"created","order_id":"100"}`),
	})
	require.NoError(t, err)
	require.Len(t, written, 1)
	require.Equal(t, "case-a-orders", written[0].Topic)

	err = harness.PlanOutbound(t.Context(), probe.OutboundExpectation{
		Topic:         "orders",
		Key:           nil,
		Headers:       nil,
		HeadersMode:   "",
		ExpectedCount: 1,
		Payload:       nil,
		PayloadSubset: []byte(`{"event":"created"}`),
		Ordering:      "",
	})
	require.NoError(t, err)

	result, err := harness.VerifyOutbound(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, result.MatchedCount)
	require.Len(t, result.ObservedMessages, 1)
	require.Equal(t, "orders", result.ObservedMessages[0].Topic)
}

// TestHarness_AwaitZeroExpectedCountWindow checks await semantics when expected_count=0.
func TestHarness_AwaitZeroExpectedCountWindow(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	writer := NewMockmessageWriter(ctrl)
	reader := NewMockmessageReader(ctrl)
	reader.EXPECT().FetchMessage(gomock.Any()).AnyTimes().Return(deadlineExceededMessage(), context.DeadlineExceeded)

	harness := newTestHarness(
		[]string{"127.0.0.1:9092"},
		"case-a",
		writer,
		nil,
		func(_ context.Context, _ string) error { return nil },
		func(_ string) (messageReader, error) { return reader, nil },
	)

	err := harness.PlanOutbound(t.Context(), probe.OutboundExpectation{
		Topic:         "orders",
		Key:           nil,
		Headers:       nil,
		HeadersMode:   "",
		ExpectedCount: 0,
		Payload:       nil,
		PayloadSubset: nil,
		Ordering:      "",
	})
	require.NoError(t, err)

	_, awaitErr := harness.AwaitOutbound(t.Context())
	require.Error(t, awaitErr)
	require.Contains(t, awaitErr.Error(), "retry window is still open")

	expiredCtx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()

	result, awaitErr := harness.AwaitOutbound(expiredCtx)
	require.NoError(t, awaitErr)
	require.Equal(t, 0, result.MatchedCount)
}

// TestHarness_CleanupBroker checks the release of reader/writer resources.
func TestHarness_CleanupBroker(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	writer := NewMockmessageWriter(ctrl)
	reader := NewMockmessageReader(ctrl)

	writer.EXPECT().Close().Times(1).Return(nil)
	reader.EXPECT().Close().Times(1).Return(nil)

	harness := newTestHarness(nil, "", writer, reader, nil, nil)

	err := harness.CleanupBroker(t.Context())
	require.NoError(t, err)
}

// TestHarness_RequirePlanErrors checks for errors when calling await/verify without PlanOutbound.
func TestHarness_RequirePlanErrors(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	writer := NewMockmessageWriter(ctrl)
	reader := NewMockmessageReader(ctrl)
	reader.EXPECT().FetchMessage(gomock.Any()).AnyTimes().Return(deadlineExceededMessage(), context.DeadlineExceeded)

	harness := newTestHarness(nil, "", writer, reader, nil, nil)

	_, awaitErr := harness.AwaitOutbound(t.Context())
	require.Error(t, awaitErr)
	require.Contains(t, awaitErr.Error(), "outbound plan is not initialized")
	_, verifyErr := harness.VerifyOutbound(t.Context())
	require.Error(t, verifyErr)
	require.Contains(t, verifyErr.Error(), "outbound plan is not initialized")
}

// deadlineExceededMessage returns an empty kafka message for "no new data" scenarios.
func deadlineExceededMessage() harnessMessage {
	return harnessMessage{
		Topic:   "",
		Key:     nil,
		Value:   nil,
		Headers: nil,
		Offset:  0,
	}
}
