// Package extapiasync demonstrates an async external API call scenario via an await step.
package extapiasync

import (
	"errors"
	"fmt"
)

// asyncStatusCode defines statuses for the async example.
type asyncStatusCode string

const (
	// asyncStatusOK means successful case execution.
	asyncStatusOK asyncStatusCode = "OK"
	// asyncStatusFailed means a failure in a step.
	asyncStatusFailed asyncStatusCode = "ERROR"
)

// asyncStatusError stores error code and message for ErrorInspector.
type asyncStatusError struct {
	code    asyncStatusCode
	message string
}

// Error returns the domain error message.
func (err asyncStatusError) Error() string {
	return err.message
}

// asyncStatusCodec converts assert.code strings into domain statuses.
type asyncStatusCodec struct{}

// Parse validates supported string status codes.
func (asyncStatusCodec) Parse(raw string) (asyncStatusCode, error) {
	switch raw {
	case string(asyncStatusOK):
		return asyncStatusOK, nil
	case string(asyncStatusFailed):
		return asyncStatusFailed, nil
	default:
		return "", fmt.Errorf("unsupported status code: %q", raw)
	}
}

// Success returns the status code for successful scenario completion.
func (asyncStatusCodec) Success() asyncStatusCode {
	return asyncStatusOK
}

// asyncErrorInspector extracts status and message from asyncStatusError.
type asyncErrorInspector struct{}

// FromError returns parsed status, message, and whether the error is recognized.
func (asyncErrorInspector) FromError(err error) (asyncStatusCode, string, bool) {
	var statusErr asyncStatusError
	if !errors.As(err, &statusErr) {
		return "", "", false
	}

	return statusErr.code, statusErr.message, true
}
