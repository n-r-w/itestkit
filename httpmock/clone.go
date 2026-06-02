// Package httpmock provides JSONC-driven outbound HTTP mocks for itestkit cases.
package httpmock

import (
	"encoding/json"
	"net/http"
	"strings"
)

// clonePlan returns a deep copy of plan fixture data.
func clonePlan(plan Plan) Plan {
	cloned := Plan{
		Calls:    make([]CallExpectation, len(plan.Calls)),
		Ordering: plan.Ordering,
	}
	for index := range plan.Calls {
		cloned.Calls[index] = cloneCall(plan.Calls[index])
	}
	return cloned
}

// cloneCall returns a deep copy of one planned call.
func cloneCall(call CallExpectation) CallExpectation {
	return CallExpectation{
		Name:           call.Name,
		ExpectedCount:  call.ExpectedCount,
		CountMode:      call.CountMode,
		Method:         call.Method,
		Path:           call.Path,
		Query:          cloneValues(call.Query),
		QueryMode:      call.QueryMode,
		Headers:        cloneValues(call.Headers),
		HeadersMode:    call.HeadersMode,
		Body:           cloneRawMessage(call.Body),
		BodySubset:     cloneRawMessage(call.BodySubset),
		RawBody:        cloneStringPointer(call.RawBody),
		FormBody:       cloneValues(call.FormBody),
		FormBodySubset: cloneValues(call.FormBodySubset),
		Response:       cloneResponse(call.Response),
	}
}

// cloneObservedRequests returns a deep copy of observed requests.
func cloneObservedRequests(requests []ObservedRequest) []ObservedRequest {
	cloned := make([]ObservedRequest, len(requests))
	for index, request := range requests {
		cloned[index] = ObservedRequest{
			Method:  request.Method,
			Path:    request.Path,
			Query:   cloneValues(request.Query),
			Headers: cloneValues(request.Headers),
			Body:    request.Body,
		}
	}
	return cloned
}

// cloneHeader returns a copy of HTTP headers with canonical header names.
func cloneHeader(header http.Header) map[string][]string {
	cloned := make(map[string][]string, len(header))
	for key, values := range header {
		cloned[http.CanonicalHeaderKey(key)] = append([]string(nil), values...)
	}
	return cloned
}

// canonicalizeHeaderValues returns a copy of headers with canonical header names.
func canonicalizeHeaderValues(headers map[string][]string) map[string][]string {
	if headers == nil {
		return nil
	}
	cloned := make(map[string][]string, len(headers))
	for key, values := range headers {
		cloned[http.CanonicalHeaderKey(strings.TrimSpace(key))] = append([]string(nil), values...)
	}
	return cloned
}

// cloneValues returns a deep copy of a repeated string map.
func cloneValues(values map[string][]string) map[string][]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string][]string, len(values))
	for key, items := range values {
		cloned[key] = append([]string(nil), items...)
	}
	return cloned
}

// cloneRawMessage returns a deep copy of raw JSON.
func cloneRawMessage(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

// cloneStringPointer returns a deep copy of a string pointer.
func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
