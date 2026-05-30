package itestkit

//go:generate go tool mockgen -source=interfaces.go -destination=interfaces_mock.go -package=itestkit

import (
	"context"
	"encoding/json"
	"io/fs"
	"testing"
)

// CaseSource describes a source for reading a tree of cases and files.
type CaseSource interface {
	ReadDir(name string) ([]fs.DirEntry, error)
	ReadFile(name string) ([]byte, error)
}

// Handler is responsible for decoding the payload, calling the handler and normalizing the response.
type Handler[C any] interface {
	// DecodeRequest converts the raw JSON of the request to the type expected by the handler.
	DecodeRequest(raw json.RawMessage) (any, error)
	// DecodeExpectedResponse converts the raw JSON of the response to the type expected by the comparison.
	DecodeExpectedResponse(raw json.RawMessage) (any, error)
	// Invoke invokes a handler with a prepared request.
	Invoke(ctx context.Context, client C, request any) (any, error)
	// NormalizeResponse brings the response into a stable form for comparison.
	NormalizeResponse(response any) (any, error)
}

// HandlerRegistry resolves handlers by name and reports supported names.
type HandlerRegistry[C any] interface {
	// Resolve returns a handler by name.
	Resolve(name string) (Handler[C], error)
	// Supported returns a list of supported handler names.
	Supported() []string
}

// HarnessFactory creates an isolated client/environment for each case.
type HarnessFactory[C any] interface {
	// New creates a new client for case execution.
	New(t *testing.T) C
}

// SuiteLifecycle manages the life cycle of a suite environment.
type SuiteLifecycle[SC any] interface {
	// SetupSuite prepares the suite environment once per set of cases.
	SetupSuite(t *testing.T) (SC, error)
	// TeardownSuite releases the suite environment after the set of cases is completed.
	TeardownSuite(t *testing.T, suiteContext SC) error
}

// SuiteCaseHarnessFactory creates a per-case harness based on the suite context.
type SuiteCaseHarnessFactory[SC any, C any] interface {
	// NewCaseHarness creates a case-level harness using the prepared suite context.
	NewCaseHarness(t *testing.T, suiteContext SC) C
}

// StatusCodec parses the status code and provides a success code.
type StatusCodec[S comparable] interface {
	// Parse converts the string code into a status type.
	Parse(raw string) (S, error)
	// Success returns the success code.
	Success() S
}

// ErrorInspector extracts the status and message from the transport error.
type ErrorInspector[S comparable] interface {
	// FromError returns the status, message, and flag whether the status was retrieved successfully.
	FromError(err error) (code S, message string, ok bool)
}
