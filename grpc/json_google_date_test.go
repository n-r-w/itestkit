//nolint:exhaustruct // dynamic protobuf descriptor fixtures in this file initialize only fields relevant to the tested decode behavior.
package grpc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// TestDecodeProtoJSONWithGoogleDateStrings_ConvertsDateString checks opt-in decode helper.
func TestDecodeProtoJSONWithGoogleDateStrings_ConvertsDateString(t *testing.T) {
	t.Parallel()

	message := dynamicpb.NewMessage(buildGoogleDateFixtureDescriptor(t))
	err := DecodeProtoJSONWithGoogleDateStrings(json.RawMessage(`{"startDate":"2026-03-11"}`), message)
	require.NoError(t, err)

	startDateField := message.Descriptor().Fields().ByName(protoreflect.Name("start_date"))
	dateMessage := message.Get(startDateField).Message()

	require.EqualValues(
		t,
		2026,
		dateMessage.Get(dateMessage.Descriptor().Fields().ByName(protoreflect.Name("year"))).Int(),
	)
	require.EqualValues(
		t,
		3,
		dateMessage.Get(dateMessage.Descriptor().Fields().ByName(protoreflect.Name("month"))).Int(),
	)
	require.EqualValues(
		t,
		11,
		dateMessage.Get(dateMessage.Descriptor().Fields().ByName(protoreflect.Name("day"))).Int(),
	)
}

// TestDecodeProtoJSONWithGoogleDateStrings_InvalidDate checks for an error on an invalid date.
func TestDecodeProtoJSONWithGoogleDateStrings_InvalidDate(t *testing.T) {
	t.Parallel()

	message := dynamicpb.NewMessage(buildGoogleDateFixtureDescriptor(t))
	err := DecodeProtoJSONWithGoogleDateStrings(json.RawMessage(`{"start_date":"bad-date"}`), message)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse google.type.Date")
}

// buildGoogleDateFixtureDescriptor builds a minimal dynamic descriptor with a `google.type.Date` field.
func buildGoogleDateFixtureDescriptor(t *testing.T) protoreflect.MessageDescriptor {
	t.Helper()

	fileDescriptor, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name:    new("google_type_date_fixture.proto"),
		Package: new("google.type"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: new("Date"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   new("year"),
						Number: proto.Int32(1),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
					},
					{
						Name:   new("month"),
						Number: proto.Int32(2),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
					},
					{
						Name:   new("day"),
						Number: proto.Int32(3),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
					},
				},
			},
			{
				Name: new("Event"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     new("start_date"),
						Number:   proto.Int32(1),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".google.type.Date"),
						JsonName: new("startDate"),
					},
				},
			},
		},
		Syntax: new("proto3"),
	}, nil)
	require.NoError(t, err)

	messageDescriptor := fileDescriptor.Messages().ByName(protoreflect.Name("Event"))
	require.NotNil(t, messageDescriptor)

	return messageDescriptor
}
