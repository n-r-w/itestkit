// Package grpc contains gRPC-specific implementations for itestkit.
package grpc

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// supportedGRPCCodes returns the complete list of supported gRPC codes.
func supportedGRPCCodes() []codes.Code {
	return []codes.Code{
		codes.OK,
		codes.Canceled,
		codes.Unknown,
		codes.InvalidArgument,
		codes.DeadlineExceeded,
		codes.NotFound,
		codes.AlreadyExists,
		codes.PermissionDenied,
		codes.ResourceExhausted,
		codes.FailedPrecondition,
		codes.Aborted,
		codes.OutOfRange,
		codes.Unimplemented,
		codes.Internal,
		codes.Unavailable,
		codes.DataLoss,
		codes.Unauthenticated,
	}
}

// grpcCodeNames returns the complete list of supported assert.code field values.
func grpcCodeNames() []string {
	supported := supportedGRPCCodes()
	names := make([]string, 0, len(supported))
	for _, code := range supported {
		names = append(names, code.String())
	}

	return names
}

// StatusCodec implements a gRPC status code parser for itestkit.
type StatusCodec struct{}

// Parse converts the string status code to codes.Code.
func (StatusCodec) Parse(raw string) (codes.Code, error) {
	for _, code := range supportedGRPCCodes() {
		if raw == code.String() {
			return code, nil
		}
	}

	return codes.Unknown, fmt.Errorf("unsupported code: %q", raw)
}

// Success returns the success code.
func (StatusCodec) Success() codes.Code {
	return codes.OK
}

// ErrorInspector extracts the code and message from a gRPC error.
type ErrorInspector struct{}

// FromError retrieves the code and message from a runtime error.
func (ErrorInspector) FromError(err error) (codes.Code, string, bool) {
	if errors.Is(err, context.Canceled) {
		return codes.Canceled, err.Error(), true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return codes.DeadlineExceeded, err.Error(), true
	}

	statusErr, ok := status.FromError(err)
	if !ok {
		return codes.Unknown, "", false
	}
	return statusErr.Code(), statusErr.Message(), true
}

// SupportedCodeNames returns a list of supported assert.code field values.
func SupportedCodeNames() []string {
	return grpcCodeNames()
}
