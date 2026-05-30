// Package queue demonstrates an event-style scenario with an in-memory queue and DB.
package queue

import (
	"errors"
	"fmt"
)

// queueStatusCode defines the minimal status set for the example.
type queueStatusCode string

const (
	queueStatusOK     queueStatusCode = "OK"
	queueStatusFailed queueStatusCode = "ERROR"
)

// queueStatusError links a runtime error to a status for assert error branches.
type queueStatusError struct {
	code    queueStatusCode
	message string
}

// Error returns a human-readable status error message.
func (err queueStatusError) Error() string {
	return err.message
}

// queueStatusCodec converts a JSONC string code to the example status type.
type queueStatusCodec struct{}

// Parse validates the case status string code.
func (queueStatusCodec) Parse(raw string) (queueStatusCode, error) {
	switch raw {
	case string(queueStatusOK):
		return queueStatusOK, nil
	case string(queueStatusFailed):
		return queueStatusFailed, nil
	default:
		return "", fmt.Errorf("unsupported status code: %q", raw)
	}
}

// Success returns the status representing successful case execution.
func (queueStatusCodec) Success() queueStatusCode {
	return queueStatusOK
}

// queueErrorInspector extracts code and message from the example domain error.
type queueErrorInspector struct{}

// FromError returns status and message when the error is a queueStatusError.
func (queueErrorInspector) FromError(err error) (queueStatusCode, string, bool) {
	var statusErr queueStatusError
	if !errors.As(err, &statusErr) {
		return "", "", false
	}

	return statusErr.code, statusErr.message, true
}
