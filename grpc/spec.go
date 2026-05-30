package grpc

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/n-r-w/itestkit"
	"github.com/n-r-w/itestkit/internal/protojsonview"
	"google.golang.org/protobuf/proto"
)

// HandlerSpec describes the handler spec contract for gRPC integration.
type HandlerSpec[C any] struct {
	NewRequest  func() proto.Message
	NewResponse func() proto.Message
	// DecodeRequestJSON allows you to override the standard proto decode for request.
	//
	// This is necessary for fixture formats that require preliminary preparation of JSON
	// before `protojson.Unmarshal`, but want to keep the common handler scaffold.
	DecodeRequestJSON func(json.RawMessage, proto.Message) error
	// DecodeExpectedResponseJSON allows you to override the decode of the expected proto-response.
	DecodeExpectedResponseJSON func(json.RawMessage, proto.Message) error
	Invoke                     func(context.Context, C, proto.Message) (proto.Message, error)
	NormalizeResponse          func(proto.Message) (any, error)
}

// NewHandlerSpec creates a type-safe handler spec.
func NewHandlerSpec[Req proto.Message, Resp proto.Message, C any](
	handlerName string,
	newRequest func() Req,
	newResponse func() Resp,
	invoke func(context.Context, C, Req) (Resp, error),
	normalize func(Resp) (any, error),
) HandlerSpec[C] {
	return HandlerSpec[C]{
		NewRequest: func() proto.Message {
			return newRequest()
		},
		NewResponse: func() proto.Message {
			return newResponse()
		},
		DecodeRequestJSON:          nil,
		DecodeExpectedResponseJSON: nil,
		Invoke: func(ctx context.Context, client C, request proto.Message) (proto.Message, error) {
			typedRequest, ok := request.(Req)
			if !ok {
				return nil, fmt.Errorf(
					"handler %s received invalid request type: got %T",
					handlerName,
					request,
				)
			}
			return invoke(ctx, client, typedRequest)
		},
		NormalizeResponse: func(response proto.Message) (any, error) {
			if normalize == nil {
				return NormalizeProtoMessage(response)
			}
			typedResponse, ok := response.(Resp)
			if !ok {
				return nil, fmt.Errorf(
					"handler %s received invalid response type: got %T",
					handlerName,
					response,
				)
			}
			return normalize(typedResponse)
		},
	}
}

// handlerAdapter adapts the HandlerSpec to the itestkit.Handler interface.
type handlerAdapter[C any] struct {
	spec HandlerSpec[C]
}

// compileTimeHandlerCheck verifies the implementation of itestkit.Handler.
var _ itestkit.Handler[any] = (*handlerAdapter[any])(nil)

// DecodeRequest decodes a JSON request into a proto-message.
func (adapter handlerAdapter[C]) DecodeRequest(raw json.RawMessage) (any, error) {
	request := adapter.spec.NewRequest()
	decodeJSON := adapter.spec.DecodeRequestJSON
	if decodeJSON == nil {
		decodeJSON = DecodeProtoJSON
	}
	if err := decodeJSON(raw, request); err != nil {
		return nil, err
	}
	return request, nil
}

// DecodeExpectedResponse decodes the expected response in the proto-message.
func (adapter handlerAdapter[C]) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	expected := adapter.spec.NewResponse()
	decodeJSON := adapter.spec.DecodeExpectedResponseJSON
	if decodeJSON == nil {
		decodeJSON = DecodeProtoJSON
	}
	if err := decodeJSON(raw, expected); err != nil {
		return nil, err
	}
	return expected, nil
}

// Invoke calls the gRPC handler.
func (adapter handlerAdapter[C]) Invoke(
	ctx context.Context,
	client C,
	request any,
) (any, error) {
	requestMessage, ok := request.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("request type %T does not implement proto.Message", request)
	}
	return adapter.spec.Invoke(ctx, client, requestMessage)
}

// NormalizeResponse normalizes the response for comparison.
func (adapter handlerAdapter[C]) NormalizeResponse(response any) (any, error) {
	responseMessage, ok := response.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("response type %T does not implement proto.Message", response)
	}
	if adapter.spec.NormalizeResponse == nil {
		return NormalizeProtoMessage(responseMessage)
	}
	return adapter.spec.NormalizeResponse(responseMessage)
}

// NormalizeProtoMessage converts the proto-message to DeepEqual-stable JSON.
//
// This allows full proto responses to be used in asserts without `reflect.DeepEqual` hooks
// on the internal details of protobuf structures.
func NormalizeProtoMessage(msg proto.Message) (any, error) {
	return protojsonview.NormalizeMessage(msg)
}
