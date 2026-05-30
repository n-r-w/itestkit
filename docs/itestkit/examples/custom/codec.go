// Package custom shows a minimal itestkit usage example without gRPC.
package custom

import (
	"errors"
	"fmt"
)

// statusCode defines the minimal set of statuses needed for the example.
type statusCode string

const (
	statusOK     statusCode = "OK"
	statusFailed statusCode = "ERROR"
)

// statusError links an execution error to a status used in assert checks.
type statusError struct {
	code    statusCode
	message string
}

// Error returns a message that can be compared by the inspector.
func (err statusError) Error() string {
	return err.message
}

// simpleStatusCodec converts a string status from a case to the local type.
type simpleStatusCodec struct{}

// Parse maps case string statuses to the typed status code.
func (simpleStatusCodec) Parse(raw string) (statusCode, error) {
	switch raw {
	case string(statusOK):
		return statusOK, nil
	case string(statusFailed):
		return statusFailed, nil
	default:
		return "", fmt.Errorf("unsupported status code: %q", raw)
	}
}

// Success returns the status code expected for a successful case.
func (simpleStatusCodec) Success() statusCode {
	return statusOK
}

// simpleErrorInspector extracts status from the example domain error.
type simpleErrorInspector struct{}

// FromError returns status and message when the error is recognized.
func (simpleErrorInspector) FromError(err error) (statusCode, string, bool) {
	var statusErr statusError
	if !errors.As(err, &statusErr) {
		return "", "", false
	}

	return statusErr.code, statusErr.message, true
}
