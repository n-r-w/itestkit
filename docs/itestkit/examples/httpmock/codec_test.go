package httpmockexample

import (
	"errors"
	"fmt"
	"strings"
)

type statusCode string

const (
	statusOK     statusCode = "OK"
	statusFailed statusCode = "ERROR"
)

type statusError struct {
	code    statusCode
	message string
}

// Error returns the status error message.
func (err statusError) Error() string {
	return err.message
}

type statusCodec struct{}

// Parse converts a JSONC status string to a status code.
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

// Success returns the success status code.
func (statusCodec) Success() statusCode {
	return statusOK
}

type errorInspector struct{}

// FromError extracts status code and message from example errors.
func (errorInspector) FromError(err error) (statusCode, string, bool) {
	var statusErr statusError
	if !errors.As(err, &statusErr) {
		return "", "", false
	}
	return statusErr.code, statusErr.message, true
}
