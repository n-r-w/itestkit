package kafkaproducer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/require"
)

// TestRunWithContext_PropagatesOperationResult verifies that an operation error is returned without mangling.
func TestRunWithContext_PropagatesOperationResult(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("operation failed")
	err := runWithContext(t.Context(), func() error {
		return expectedErr
	})
	require.ErrorIs(t, err, expectedErr)
}

// TestRunWithContext_CanceledBeforeOperation checks the priority of canceling a context when the ctx has already been canceled.
func TestRunWithContext_CanceledBeforeOperation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	releaseCh := make(chan struct{})

	err := runWithContext(ctx, func() error {
		<-releaseCh
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)
	close(releaseCh)
}

// TestRunWithContext_CanceledDuringOperation checks for early exit when a context is canceled during a blocking operation.
func TestRunWithContext_CanceledDuringOperation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	startedCh := make(chan struct{})
	releaseCh := make(chan struct{})
	errCh := make(chan error, 1)

	go func() {
		errCh <- runWithContext(ctx, func() error {
			close(startedCh)
			<-releaseCh
			return nil
		})
	}()

	select {
	case <-startedCh:
	case <-time.After(time.Second):
		t.Fatal("operation did not start in time")
	}

	cancel()
	select {
	case err := <-errCh:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("runWithContext did not exit after context cancellation")
	}
	close(releaseCh)
}

// TestIsTopicAlreadyExistsError tests the error detection of an existing topic.
func TestIsTopicAlreadyExistsError(t *testing.T) {
	t.Parallel()

	require.False(t, isTopicAlreadyExistsError(nil))
	require.True(t, isTopicAlreadyExistsError(sarama.ErrTopicAlreadyExists))
	require.True(t, isTopicAlreadyExistsError(errors.New("topic ALREADY EXISTS")))
	require.False(t, isTopicAlreadyExistsError(errors.New("unknown error")))
}
