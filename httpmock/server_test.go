package httpmock

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/n-r-w/itestkit"
)

// TestServer_ReturnsPlannedResponse checks the main plan-record-verify flow through a real HTTP client.
func TestServer_ReturnsPlannedResponse(t *testing.T) {
	t.Parallel()

	call := testCall(http.MethodPost, "/charges", testResponse(http.StatusCreated))
	call.Name = "create-charge"
	call.Query = map[string][]string{"dry_run": {"false"}}
	call.QueryMode = QueryModeExact
	call.Headers = map[string][]string{"X-Request-ID": {"request-1"}}
	call.HeadersMode = HeadersModeSubset
	call.Body = rawJSON(t, `{"order_id":"order-1","amount":100}`)
	call.Response.Headers = map[string][]string{"Content-Type": {"application/json"}}
	call.Response.Body = rawJSON(t, `{"result":"approved"}`)

	server := NewServer(t)
	require.NoError(t, server.Plan(t.Context(), testPlan(OrderingAny, call)))

	request, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		server.URL()+"/charges?dry_run=false",
		bytes.NewBufferString(`{"amount":100,"order_id":"order-1"}`),
	)
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Request-ID", "request-1")

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, response.Body.Close())
	})
	responseBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusCreated, response.StatusCode)
	assert.JSONEq(t, `{"result":"approved"}`, string(responseBody))

	result, err := server.Verify(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, result.MatchedCount)
	require.Len(t, result.ObservedRequests, 1)
	assert.Equal(t, http.MethodPost, result.ObservedRequests[0].Method)
	assert.Equal(t, "/charges", result.ObservedRequests[0].Path)
}

// TestServer_VerifyReportsMismatch checks that request mismatches fail final verification.
func TestServer_VerifyReportsMismatch(t *testing.T) {
	t.Parallel()

	call := testCall(http.MethodPost, "/charges", testResponse(http.StatusOK))
	call.BodySubset = rawJSON(t, `{"order_id":"order-1"}`)

	server := NewServer(t)
	require.NoError(t, server.Plan(t.Context(), testPlan("", call)))

	request, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		server.URL()+"/charges",
		bytes.NewBufferString(`{"order_id":"order-2"}`),
	)
	require.NoError(t, err)

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, response.Body.Close())
	})

	assert.Equal(t, http.StatusInternalServerError, response.StatusCode)
	result, verifyErr := server.Verify(t.Context())
	require.Error(t, verifyErr)
	assert.Equal(t, 0, result.MatchedCount)
	assert.Contains(t, verifyErr.Error(), "method mismatch")
}

// TestServer_HeadersPresentAcceptsDynamicValues checks required headers without comparing generated values.
func TestServer_HeadersPresentAcceptsDynamicValues(t *testing.T) {
	t.Parallel()

	// ARRANGE: the plan checks one fixed header value and two dynamic headers by presence only.
	plan := decodePlan(t, `{
		"calls": [
			{
				"expected_count": 1,
				"method": "POST",
				"path": "/charges",
				"headers_mode": "subset",
				"headers": {"X-Service-ID": ["admin"]},
				"headers_present": ["X-Timestamp", "X-Signature"],
				"response": {"status": 204}
			}
		]
	}`)
	server := NewServer(t)
	require.NoError(t, server.Plan(t.Context(), plan))

	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL()+"/charges", http.NoBody)
	require.NoError(t, err)
	request.Header.Set("X-Service-ID", "admin")
	request.Header.Set("x-timestamp", "2026-06-28T12:00:00Z")
	request.Header.Set("X-Signature", "generated-signature")

	// ACT: the request matches because required dynamic headers are present.
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, response.Body.Close())
	})
	result, verifyErr := server.Verify(t.Context())

	// ASSERT: value matching and presence-only matching work together.
	assert.Equal(t, http.StatusNoContent, response.StatusCode)
	require.NoError(t, verifyErr)
	assert.Equal(t, 1, result.MatchedCount)
}

// TestServer_HeadersPresentReportsMissingHeader checks that presence-only headers are still required.
func TestServer_HeadersPresentReportsMissingHeader(t *testing.T) {
	t.Parallel()

	// ARRANGE: the plan requires two dynamic headers but the request will send only one of them.
	plan := decodePlan(t, `{
		"calls": [
			{
				"expected_count": 1,
				"method": "POST",
				"path": "/charges",
				"headers_present": ["X-Timestamp", "X-Signature"],
				"response": {"status": 204}
			}
		]
	}`)
	server := NewServer(t)
	require.NoError(t, server.Plan(t.Context(), plan))

	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL()+"/charges", http.NoBody)
	require.NoError(t, err)
	request.Header.Set("X-Timestamp", "2026-06-28T12:00:00Z")

	// ACT: the server records the request, but matcher rejects the missing required header.
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, response.Body.Close())
	})
	result, verifyErr := server.Verify(t.Context())

	// ASSERT: the mismatch names the absent header so fixture failures are actionable.
	assert.Equal(t, http.StatusInternalServerError, response.StatusCode)
	require.Error(t, verifyErr)
	assert.Equal(t, 0, result.MatchedCount)
	assert.Contains(t, verifyErr.Error(), "headers_present mismatch")
	assert.Contains(t, verifyErr.Error(), "X-Signature")
}

// TestServer_PassThroughDelegatesToWrappedHandler checks that the recorder keeps the request body available for the wrapped handler.
func TestServer_PassThroughDelegatesToWrappedHandler(t *testing.T) {
	t.Parallel()

	call := testCall(http.MethodPost, "/charges", testResponse(http.StatusOK))
	call.BodySubset = rawJSON(t, `{"order_id":"order-1"}`)

	server := NewPassThroughServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			return
		}
		assert.JSONEq(t, `{"order_id":"order-1","amount":100}`, string(body))
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusAccepted)
		_, _ = writer.Write([]byte(`{"from":"wrapped"}`))
	}))
	require.NoError(t, server.Plan(t.Context(), testPlan("", call)))

	request, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		server.URL()+"/charges",
		bytes.NewBufferString(`{"order_id":"order-1","amount":100}`),
	)
	require.NoError(t, err)

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, response.Body.Close())
	})
	responseBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusAccepted, response.StatusCode)
	assert.JSONEq(t, `{"from":"wrapped"}`, string(responseBody))
	result, err := server.Verify(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, result.MatchedCount)
}

// TestServer_FormBodySubset checks order-insensitive matching for form-encoded request bodies.
func TestServer_FormBodySubset(t *testing.T) {
	t.Parallel()

	call := testCall(http.MethodPost, "/pay", testResponse(http.StatusOK))
	call.FormBodySubset = map[string][]string{"OPERATION": {"Init"}, "AMOUNT": {"1000"}}
	call.Response.RawBody = stringPtr("RESULT=0")

	server := NewServer(t)
	require.NoError(t, server.Plan(t.Context(), testPlan("", call)))

	request, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		server.URL()+"/pay",
		bytes.NewBufferString("AMOUNT=1000&EXTRA=1&OPERATION=Init"),
	)
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, response.Body.Close())
	})

	assert.Equal(t, http.StatusOK, response.StatusCode)
	result, err := server.Verify(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, result.MatchedCount)
}

// TestServer_RawBodyAndQuerySubset checks raw body matching and order-insensitive query subset matching.
func TestServer_RawBodyAndQuerySubset(t *testing.T) {
	t.Parallel()

	call := testCall(http.MethodPost, "/raw", testResponse(http.StatusAccepted))
	call.Query = map[string][]string{"tag": {"b", "a"}}
	call.QueryMode = QueryModeSubset
	call.RawBody = stringPtr("plain body")
	call.Response.RawBody = stringPtr("accepted")

	server := NewServer(t)
	require.NoError(t, server.Plan(t.Context(), testPlan("", call)))

	request, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		server.URL()+"/raw?extra=1&tag=a&tag=b",
		bytes.NewBufferString("plain body"),
	)
	require.NoError(t, err)
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, response.Body.Close())
	})
	responseBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusAccepted, response.StatusCode)
	assert.Equal(t, "accepted", string(responseBody))
	_, verifyErr := server.Verify(t.Context())
	require.NoError(t, verifyErr)
}

// TestServer_CountModeAtLeastAllowsExtraMatchingRequests checks polling-style request verification.
func TestServer_CountModeAtLeastAllowsExtraMatchingRequests(t *testing.T) {
	t.Parallel()

	call := testCall(http.MethodGet, "/status", testResponse(http.StatusOK))
	call.ExpectedCount = 1
	call.CountMode = CountModeAtLeast

	server := NewServer(t)
	require.NoError(t, server.Plan(t.Context(), testPlan("", call)))

	getURL(t, server.URL()+"/status")
	getURL(t, server.URL()+"/status")

	result, verifyErr := server.Verify(t.Context())
	require.NoError(t, verifyErr)
	assert.Equal(t, 2, result.MatchedCount)
}

// TestServer_ExpectedCountZeroRejectsMatchingRequest checks that zero-count expectations reject matching calls.
func TestServer_ExpectedCountZeroRejectsMatchingRequest(t *testing.T) {
	t.Parallel()

	call := testCall(http.MethodGet, "/forbidden", testResponse(http.StatusOK))
	call.ExpectedCount = 0

	server := NewServer(t)
	require.NoError(t, server.Plan(t.Context(), testPlan("", call)))

	responseBody, statusCode := getURLBody(t, server.URL()+"/forbidden")

	assert.Equal(t, http.StatusInternalServerError, statusCode)
	assert.Equal(t, unexpectedRequestBody, responseBody)
	_, verifyErr := server.Verify(t.Context())
	require.Error(t, verifyErr)
	assert.Contains(t, verifyErr.Error(), "unexpected request")
}

// TestServer_AwaitEvaluatesCanceledContext checks runner-compatible final await evaluation.
func TestServer_AwaitEvaluatesCanceledContext(t *testing.T) {
	t.Parallel()

	server := NewServer(t)
	require.NoError(t, server.Plan(t.Context(), testPlan("", testCall(http.MethodGet, "/ready", testResponse(http.StatusOK)))))
	getURL(t, server.URL()+"/ready")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result, awaitErr := server.Await(ctx)

	require.NoError(t, awaitErr)
	assert.Equal(t, 1, result.MatchedCount)
}

// TestServer_Ordering checks strict and any-order verification rules.
func TestServer_Ordering(t *testing.T) {
	t.Parallel()

	t.Run("strict fails on reversed calls", func(t *testing.T) {
		t.Parallel()

		server := NewServer(t)
		require.NoError(t, server.Plan(t.Context(), twoCallPlan(OrderingStrict)))

		getURL(t, server.URL()+"/second")
		getURL(t, server.URL()+"/first")

		_, verifyErr := server.Verify(t.Context())
		require.Error(t, verifyErr)
		assert.Contains(t, verifyErr.Error(), "strict ordering mismatch")
	})

	t.Run("any passes on reversed calls", func(t *testing.T) {
		t.Parallel()

		server := NewServer(t)
		require.NoError(t, server.Plan(t.Context(), twoCallPlan(OrderingAny)))

		getURL(t, server.URL()+"/second")
		getURL(t, server.URL()+"/first")

		result, verifyErr := server.Verify(t.Context())
		require.NoError(t, verifyErr)
		assert.Equal(t, 2, result.MatchedCount)
	})
}

// TestServer_RejectsOversizedRequestBody checks the bounded request body guard.
func TestServer_RejectsOversizedRequestBody(t *testing.T) {
	t.Parallel()

	call := testCall(http.MethodPost, "/upload", testResponse(http.StatusOK))
	call.RawBody = stringPtr("small")

	server := NewServer(t, WithMaxRequestBodyBytes(4))
	require.NoError(t, server.Plan(t.Context(), testPlan("", call)))

	request, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		server.URL()+"/upload",
		bytes.NewBufferString("too large"),
	)
	require.NoError(t, err)

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, response.Body.Close())
	})

	assert.Equal(t, http.StatusRequestEntityTooLarge, response.StatusCode)
}

// TestNewRegistry_IncludesPresetAndCustom checks registry composition.
func TestNewRegistry_IncludesPresetAndCustom(t *testing.T) {
	t.Parallel()

	registry := NewRegistry[testHarness](map[string]itestkit.Handler[testHarness]{
		"CustomAction": stubHandler[testHarness]{},
	})

	assert.ElementsMatch(t, []string{
		PlanHTTPCallsHandlerName,
		AwaitHTTPCallsHandlerName,
		VerifyHTTPCallsHandlerName,
		"CustomAction",
	}, registry.Supported())
}

type testHarness struct {
	server *Server
}

// HTTPMock returns the per-case HTTP mock server.
func (harness testHarness) HTTPMock() *Server {
	return harness.server
}

type stubHandler[C any] struct{}

var _ itestkit.Handler[testHarness] = stubHandler[testHarness]{}

// DecodeRequest returns an empty request for the registry composition test.
func (stubHandler[C]) DecodeRequest(_ json.RawMessage) (any, error) {
	return struct{}{}, nil
}

// DecodeExpectedResponse returns an empty response for the registry composition test.
func (stubHandler[C]) DecodeExpectedResponse(_ json.RawMessage) (any, error) {
	return struct{}{}, nil
}

// Invoke returns an empty response for the registry composition test.
func (stubHandler[C]) Invoke(_ context.Context, _ C, _ any) (any, error) {
	return struct{}{}, nil
}

// NormalizeResponse returns the response unchanged for the registry composition test.
func (stubHandler[C]) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// rawJSON returns a JSON raw message after checking that the fixture is valid JSON.
func rawJSON(t *testing.T, value string) json.RawMessage {
	t.Helper()

	var decoded any
	require.NoError(t, json.Unmarshal([]byte(value), &decoded))
	return json.RawMessage(value)
}

// decodePlan decodes JSONC-like test JSON through the same strict path as PlanHTTPCalls.
func decodePlan(t *testing.T, value string) Plan {
	t.Helper()

	handler := PlanHTTPCallsHandler[testHarness]{}
	decoded, err := handler.DecodeRequest(rawJSON(t, value))
	require.NoError(t, err)
	plan, ok := decoded.(Plan)
	require.True(t, ok)
	return plan
}

// stringPtr returns a pointer to value.
func stringPtr(value string) *string {
	return &value
}

// getURL sends a GET request and closes the response body.
func getURL(t *testing.T, url string) {
	t.Helper()

	_, _ = getURLBody(t, url)
}

// getURLBody sends a GET request, reads the response body, and closes it.
func getURLBody(t *testing.T, url string) (string, int) {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
	require.NoError(t, err)
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	return string(body), response.StatusCode
}

// twoCallPlan returns a two-request plan for ordering tests.
func twoCallPlan(ordering Ordering) Plan {
	return testPlan(
		ordering,
		testCall(http.MethodGet, "/first", testResponse(http.StatusOK)),
		testCall(http.MethodGet, "/second", testResponse(http.StatusOK)),
	)
}

// testPlan returns a plan with all fields initialized for lint compatibility.
func testPlan(ordering Ordering, calls ...CallExpectation) Plan {
	return Plan{Calls: calls, Ordering: ordering}
}

// testCall returns a call expectation with all fields initialized for lint compatibility.
func testCall(method, path string, response Response) CallExpectation {
	return CallExpectation{
		Name:           "",
		ExpectedCount:  1,
		CountMode:      "",
		Method:         method,
		Path:           path,
		Query:          nil,
		QueryMode:      "",
		Headers:        nil,
		HeadersMode:    "",
		HeadersPresent: nil,
		Body:           nil,
		BodySubset:     nil,
		RawBody:        nil,
		FormBody:       nil,
		FormBodySubset: nil,
		Response:       response,
	}
}

// testResponse returns a response with all fields initialized for lint compatibility.
func testResponse(status int) Response {
	return Response{Status: status, Headers: nil, Body: nil, RawBody: nil}
}
