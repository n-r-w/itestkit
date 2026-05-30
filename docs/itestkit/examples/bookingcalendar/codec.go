// Package bookingcalendar demonstrates calendar-aware JSONC fixtures with a fixed application clock.
package bookingcalendar

import (
	"errors"
	"fmt"
)

// bookingStatusCode defines the minimal status set used by the example.
type bookingStatusCode string

const (
	// bookingStatusOK means successful case completion.
	bookingStatusOK bookingStatusCode = "OK"
	// bookingStatusFailed means that a step returned an execution error.
	bookingStatusFailed bookingStatusCode = "ERROR"
)

// bookingStatusError links an execution error to the example status model.
type bookingStatusError struct {
	code    bookingStatusCode
	message string
}

// newBookingStatusError keeps example failures easy to read without repeating status boilerplate.
func newBookingStatusError(format string, args ...any) bookingStatusError {
	return bookingStatusError{
		code:    bookingStatusFailed,
		message: fmt.Sprintf(format, args...),
	}
}

// Error returns the text that is later inspected by ErrorInspector.
func (err bookingStatusError) Error() string {
	return err.message
}

// bookingStatusCodec converts JSONC assert.code values into typed statuses.
type bookingStatusCodec struct{}

// Parse validates supported status codes used by the example fixtures.
func (bookingStatusCodec) Parse(raw string) (bookingStatusCode, error) {
	switch raw {
	case string(bookingStatusOK):
		return bookingStatusOK, nil
	case string(bookingStatusFailed):
		return bookingStatusFailed, nil
	default:
		return "", fmt.Errorf("unsupported status code: %q", raw)
	}
}

// Success returns the status that means successful scenario execution.
func (bookingStatusCodec) Success() bookingStatusCode {
	return bookingStatusOK
}

// bookingErrorInspector extracts status and message from domain errors.
type bookingErrorInspector struct{}

// FromError returns details only for errors produced by the example client.
func (bookingErrorInspector) FromError(err error) (bookingStatusCode, string, bool) {
	var statusErr bookingStatusError
	if !errors.As(err, &statusErr) {
		return "", "", false
	}

	return statusErr.code, statusErr.message, true
}
