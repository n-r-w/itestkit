// Package protojsonview converts protobuf messages into JSON-like values.
package protojsonview

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// NormalizeMessage casts proto.Message to a JSON-like value.
func NormalizeMessage(msg proto.Message) (any, error) {
	if msg == nil {
		return nil, errors.New("normalize proto message: nil message")
	}

	var marshalOptions protojson.MarshalOptions
	marshalOptions.UseProtoNames = true

	raw, err := marshalOptions.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("normalize proto message: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	var normalized any
	if err = decoder.Decode(&normalized); err != nil {
		return nil, fmt.Errorf("normalize proto message: %w", err)
	}

	return normalized, nil
}
