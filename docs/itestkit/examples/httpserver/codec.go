package httpserverexample

import (
	"errors"
	"fmt"
	"strings"
)

// statusCode defines example-level execution status for itestkit assertions.
type statusCode string

const (
	// statusOK means a case executed without harness-level errors.
	statusOK statusCode = "OK"
	// statusFailed means a case failed before a comparable HTTP response was produced.
	statusFailed statusCode = "ERROR"
)

// statusError links an execution error to a status used in assert checks.
type statusError struct {
	code    statusCode
	message string
}

// Error returns the status error message.
func (err statusError) Error() string {
	return err.message
}

// statusCodec converts JSONC assert.code values to example status values.
type statusCodec struct{}

// Parse validates and converts a JSONC status string.
func (statusCodec) Parse(raw string) (statusCode, error) {
	switch strings.TrimSpace(raw) {
	case string(statusOK):
		return statusOK, nil
	case string(statusFailed):
		return statusFailed, nil
	default:
		return "", fmt.Errorf("unsupported status code %q", raw)
	}
}

// Success returns the status expected for successful case execution.
func (statusCodec) Success() statusCode {
	return statusOK
}

// errorInspector extracts status code and message from example errors.
type errorInspector struct{}

// FromError returns status and message when the error is recognized by this example.
func (errorInspector) FromError(err error) (statusCode, string, bool) {
	var statusErr statusError
	if !errors.As(err, &statusErr) {
		return "", "", false
	}
	return statusErr.code, statusErr.message, true
}
