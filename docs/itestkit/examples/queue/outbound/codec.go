// Package kafkaoutbound demonstrates a ready-to-use outbound Kafka preset for JSONC cases.
package kafkaoutbound

import (
	"fmt"
	"strings"
)

// outboundStatus defines test status codes for kafkaoutbound example.
type outboundStatus string

const (
	// outboundStatusOK indicates successful case execution.
	outboundStatusOK outboundStatus = "OK"
	// outboundStatusError indicates expected failing case execution.
	outboundStatusError outboundStatus = "ERROR"
)

// outboundStatusCodec converts string status from JSONC into domain type.
type outboundStatusCodec struct{}

// Parse parses case status code string.
func (outboundStatusCodec) Parse(raw string) (outboundStatus, error) {
	normalized := strings.ToUpper(strings.TrimSpace(raw))
	switch normalized {
	case string(outboundStatusOK):
		return outboundStatusOK, nil
	case string(outboundStatusError):
		return outboundStatusError, nil
	default:
		return outboundStatusError, fmt.Errorf("unsupported status %q", raw)
	}
}

// Success returns success status code.
func (outboundStatusCodec) Success() outboundStatus {
	return outboundStatusOK
}

// outboundErrorInspector extracts code and message from runtime error.
type outboundErrorInspector struct{}

// FromError maps runtime error to ERROR status.
func (outboundErrorInspector) FromError(err error) (code outboundStatus, message string, ok bool) {
	if err == nil {
		return outboundStatusError, "", false
	}

	return outboundStatusError, err.Error(), true
}
