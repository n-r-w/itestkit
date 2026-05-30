// Package extapisync demonstrates a synchronous external API call scenario in a step pipeline.
package extapisync

import (
	"errors"
	"fmt"
)

// syncStatusCode defines the minimal status set for example scenarios.
type syncStatusCode string

const (
	// syncStatusOK means successful case completion.
	syncStatusOK syncStatusCode = "OK"
	// syncStatusFailed means a step execution error.
	syncStatusFailed syncStatusCode = "ERROR"
)

// syncStatusError links an execution error to a status for ErrorInspector.
type syncStatusError struct {
	code    syncStatusCode
	message string
}

// Error returns a human-readable error message.
func (err syncStatusError) Error() string {
	return err.message
}

// syncStatusCodec converts a JSONC string code into a domain status.
type syncStatusCodec struct{}

// Parse validates supported status string codes.
func (syncStatusCodec) Parse(raw string) (syncStatusCode, error) {
	switch raw {
	case string(syncStatusOK):
		return syncStatusOK, nil
	case string(syncStatusFailed):
		return syncStatusFailed, nil
	default:
		return "", fmt.Errorf("unsupported status code: %q", raw)
	}
}

// Success returns the status code that means successful scenario execution.
func (syncStatusCodec) Success() syncStatusCode {
	return syncStatusOK
}

// syncErrorInspector extracts code and message from the example domain error.
type syncErrorInspector struct{}

// FromError returns error details when the error is a syncStatusError.
func (syncErrorInspector) FromError(err error) (syncStatusCode, string, bool) {
	var statusErr syncStatusError
	if !errors.As(err, &statusErr) {
		return "", "", false
	}

	return statusErr.code, statusErr.message, true
}
