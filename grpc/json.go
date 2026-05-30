package grpc

import (
	"encoding/json"
	"errors"
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// DecodeProtoJSON decodes JSON into proto-message and disables unknown fields.
//
// This is the basic decode path for gRPC helpers. For non-standard form fixture JSON
// it can be wrapped in a custom decode hook at the `HandlerSpec` level.
func DecodeProtoJSON(raw json.RawMessage, msg proto.Message) error {
	if msg == nil {
		return errors.New("decode proto json: nil message")
	}
	var unmarshalOptions protojson.UnmarshalOptions
	if err := unmarshalOptions.Unmarshal(raw, msg); err != nil {
		return fmt.Errorf("decode proto json: %w", err)
	}
	return nil
}
