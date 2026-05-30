// Package itest contains ready-made preset handlers for outbound checks in the itestkit pipeline.
package itest

//go:generate go tool mockgen -source=interfaces.go -destination=interfaces_mock.go -package=itest

import (
	"context"

	"github.com/n-r-w/itestkit/queue/probe"
)

// OutboundHarness describes the minimum harness contract for outbound scenarios.
type OutboundHarness interface {
	// PlanOutbound saves expectations and activates the observation boundary for the current case.
	PlanOutbound(ctx context.Context, expectation probe.OutboundExpectation) error
	// AwaitOutbound performs an intermediate check of expectations in the await step.
	AwaitOutbound(ctx context.Context) (probe.CheckResult, error)
	// VerifyOutbound performs the final check of expectations in the verify step.
	VerifyOutbound(ctx context.Context) (probe.CheckResult, error)
	// CleanupBroker releases helper resources for the current case.
	CleanupBroker(ctx context.Context) error
}
