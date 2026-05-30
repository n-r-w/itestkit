package grpc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// DecodeProtoJSONWithGoogleDateStrings decodes fixture JSON into proto-message,
// where the `google.type.Date` fields can be specified with the string `YYYY-MM-DD`.
//
// This is an opt-in helper for human-readable fixtures. It doesn't change behavior
// base `DecodeProtoJSON(...)` by default.
func DecodeProtoJSONWithGoogleDateStrings(raw json.RawMessage, msg proto.Message) error {
	if msg == nil {
		return errors.New("decode proto json with google date strings: nil message")
	}
	if len(raw) == 0 {
		return nil
	}

	normalizedRaw, err := normalizeProtoJSONWithGoogleDateStrings(raw, msg.ProtoReflect().Descriptor())
	if err != nil {
		return fmt.Errorf("prepare proto json with google date strings: %w", err)
	}

	decodeErr := DecodeProtoJSON(normalizedRaw, msg)
	if decodeErr != nil {
		return decodeErr
	}

	return nil
}

// decodeJSONValueWithNumbers decodes JSON to `any` and stores number tokens as `json.Number`.
//
// This is necessary so that intermediate normalization does not lose the accuracy of numeric fields,
// for now we are only rewriting the necessary fragments of the fixture JSON.
func decodeJSONValueWithNumbers(raw json.RawMessage) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("decode json: %w", err)
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("decode json: trailing data")
	}

	return value, nil
}

// normalizeProtoJSONWithGoogleDateStrings prepares fixture JSON to `protojson.Unmarshal`.
func normalizeProtoJSONWithGoogleDateStrings(
	raw json.RawMessage,
	descriptor protoreflect.MessageDescriptor,
) ([]byte, error) {
	value, err := decodeJSONValueWithNumbers(raw)
	if err != nil {
		return nil, err
	}

	normalizedValue, err := normalizeProtoMessageValueWithGoogleDateStrings(value, descriptor)
	if err != nil {
		return nil, err
	}

	normalizedRaw, err := json.Marshal(normalizedValue)
	if err != nil {
		return nil, fmt.Errorf("marshal normalized proto json: %w", err)
	}

	return normalizedRaw, nil
}

// normalizeProtoMessageValueWithGoogleDateStrings recursively produces the JSON representation of the message
// to a form that understands `protojson.Unmarshal` for `google.type.Date`.
func normalizeProtoMessageValueWithGoogleDateStrings(
	value any,
	descriptor protoreflect.MessageDescriptor,
) (any, error) {
	objectValue, ok := value.(map[string]any)
	if !ok {
		return value, nil
	}

	normalizedObject := make(map[string]any, len(objectValue))
	for key, fieldValue := range objectValue {
		fieldDescriptor := findProtoFieldByJSONKey(descriptor, key)
		if fieldDescriptor == nil {
			normalizedObject[key] = fieldValue
			continue
		}

		normalizedFieldValue, err := normalizeProtoFieldValueWithGoogleDateStrings(fieldValue, fieldDescriptor)
		if err != nil {
			return nil, fmt.Errorf("normalize field %q: %w", key, err)
		}

		normalizedObject[key] = normalizedFieldValue
	}

	return normalizedObject, nil
}

// normalizeProtoFieldValueWithGoogleDateStrings recursively normalizes the value of a protobuf field.
func normalizeProtoFieldValueWithGoogleDateStrings(
	value any,
	fieldDescriptor protoreflect.FieldDescriptor,
) (any, error) {
	if value == nil {
		return json.RawMessage{}, nil
	}

	if fieldDescriptor.IsList() {
		listValue, ok := value.([]any)
		if !ok {
			return value, nil
		}

		normalizedList := make([]any, 0, len(listValue))
		for _, itemValue := range listValue {
			normalizedItemValue, err := normalizeProtoSingularValueWithGoogleDateStrings(itemValue, fieldDescriptor)
			if err != nil {
				return nil, err
			}

			normalizedList = append(normalizedList, normalizedItemValue)
		}

		return normalizedList, nil
	}

	return normalizeProtoSingularValueWithGoogleDateStrings(value, fieldDescriptor)
}

// normalizeProtoSingularValueWithGoogleDateStrings converts a single protobuf field value.
func normalizeProtoSingularValueWithGoogleDateStrings(
	value any,
	fieldDescriptor protoreflect.FieldDescriptor,
) (any, error) {
	if fieldDescriptor.Kind() != protoreflect.MessageKind {
		return value, nil
	}

	if fieldDescriptor.Message().FullName() == "google.type.Date" {
		dateString, ok := value.(string)
		if !ok {
			return value, nil
		}

		return normalizeGoogleDateString(dateString)
	}

	return normalizeProtoMessageValueWithGoogleDateStrings(value, fieldDescriptor.Message())
}

// findProtoFieldByJSONKey finds a field by snake_case and lowerCamel JSON key.
func findProtoFieldByJSONKey(
	descriptor protoreflect.MessageDescriptor,
	jsonKey string,
) protoreflect.FieldDescriptor {
	fields := descriptor.Fields()
	for index := range fields.Len() {
		fieldDescriptor := fields.Get(index)
		if string(fieldDescriptor.Name()) == jsonKey || fieldDescriptor.JSONName() == jsonKey {
			return fieldDescriptor
		}
	}

	return nil
}

// normalizeGoogleDateString translates the string `YYYY-MM-DD` into a JSON object `google.type.Date`.
func normalizeGoogleDateString(value string) (map[string]any, error) {
	parsedDate, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil, fmt.Errorf("parse google.type.Date: %w", err)
	}

	return map[string]any{
		"year":  parsedDate.Year(),
		"month": int(parsedDate.Month()),
		"day":   parsedDate.Day(),
	}, nil
}
