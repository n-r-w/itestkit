package httpserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testHarness owns the in-process HTTP handler and per-case cookie jar used by the tests.
type testHarness struct {
	handler http.Handler
	jar     *CookieJar
}

// HTTPHandler returns the handler under test for CallHandler.
func (harness *testHarness) HTTPHandler() http.Handler {
	return harness.handler
}

// HTTPCookieJar returns per-case cookies that CallHandler may attach to later requests.
func (harness *testHarness) HTTPCookieJar() *CookieJar {
	return harness.jar
}

// TestCallHandlerInvokesHTTPHandler verifies request construction and normalized JSON response output.
func TestCallHandlerInvokesHTTPHandler(t *testing.T) {
	t.Parallel()

	// ARRANGE: the HTTP handler checks the transport-level request created from JSONC fields.
	harness := &testHarness{handler: nil, jar: nil}
	harness.handler = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, http.MethodPost, request.Method)
		assert.Equal(t, "api.test", request.Host)
		assert.Equal(t, "/v1/orders", request.URL.Path)
		assert.Equal(t, "new", request.URL.Query().Get("kind"))
		assert.Equal(t, "application/json", request.Header.Get("Content-Type"))
		assert.Equal(t, "req-1", request.Header.Get("X-Request-ID"))

		rawBody, err := io.ReadAll(request.Body)
		if !assert.NoError(t, err) {
			return
		}
		assert.JSONEq(t, `{"name":"coffee"}`, string(rawBody))

		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("X-Trace", "trace-1")
		writer.WriteHeader(http.StatusCreated)
		_, err = writer.Write([]byte(`{"id":123,"ok":true}`))
		assert.NoError(t, err)
	})

	callHandler := NewCallHandler[*testHarness](WithBaseURL("http://api.test"))
	rawRequest := json.RawMessage(`{
		"method": "POST",
		"path": "/v1/orders",
		"query": {"kind": "new"},
		"headers": {"Content-Type": "application/json", "X-Request-ID": "req-1"},
		"body": {"name": "coffee"},
		"capture_headers": ["X-Trace"]
	}`)

	// ACT: decode the fixture request, invoke the handler, and normalize the response for comparison.
	request, err := callHandler.DecodeRequest(rawRequest)
	require.NoError(t, err)
	response, err := callHandler.Invoke(t.Context(), harness, request)
	require.NoError(t, err)
	normalized, err := callHandler.NormalizeResponse(response)
	require.NoError(t, err)

	// ASSERT: the normalized response contains only stable fields controlled by the fixture contract.
	assert.Equal(t, map[string]any{
		"status": http.StatusCreated,
		"headers": map[string][]string{
			"X-Trace": {"trace-1"},
		},
		"body": map[string]any{
			"id": float64(123),
			"ok": true,
		},
	}, normalized)
}

// TestCallHandlerKeepsCookiesInCaseJar verifies explicit cookie reuse between case steps.
func TestCallHandlerKeepsCookiesInCaseJar(t *testing.T) {
	t.Parallel()

	// ARRANGE: one endpoint issues a session cookie and another endpoint requires it.
	harness := &testHarness{handler: nil, jar: NewCookieJar()}
	harness.handler = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")

		switch request.URL.Path {
		case "/login":
			cookie := new(http.Cookie)
			cookie.Name = "session_id"
			cookie.Value = "abc"
			cookie.Path = "/"
			cookie.HttpOnly = true
			cookie.SameSite = http.SameSiteLaxMode
			http.SetCookie(writer, cookie)
			_, err := writer.Write([]byte(`{"logged_in":true}`))
			assert.NoError(t, err)
		case "/me":
			cookie, err := request.Cookie("session_id")
			if !assert.NoError(t, err) {
				return
			}
			_, err = writer.Write([]byte(`{"session":"` + cookie.Value + `"}`))
			assert.NoError(t, err)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	})

	callHandler := NewCallHandler[*testHarness]()

	// ACT: the first step stores the cookie and exposes selected cookie fields for assertion.
	loginRequest, err := callHandler.DecodeRequest(json.RawMessage(`{
		"method": "POST",
		"path": "/login",
		"capture_cookies": ["session_id"]
	}`))
	require.NoError(t, err)
	loginResponse, err := callHandler.Invoke(t.Context(), harness, loginRequest)
	require.NoError(t, err)
	loginNormalized, err := callHandler.NormalizeResponse(loginResponse)
	require.NoError(t, err)

	// ACT: the second step explicitly attaches cookies stored by the previous response.
	meRequest, err := callHandler.DecodeRequest(json.RawMessage(`{
		"method": "GET",
		"path": "/me",
		"use_cookies": true
	}`))
	require.NoError(t, err)
	meResponse, err := callHandler.Invoke(t.Context(), harness, meRequest)
	require.NoError(t, err)
	meNormalized, err := callHandler.NormalizeResponse(meResponse)
	require.NoError(t, err)

	// ASSERT: captured cookie attributes are stable and later requests use the stored cookie value.
	assert.Equal(t, map[string]any{
		"status": http.StatusOK,
		"cookies": map[string]map[string]any{
			"session_id": {
				"value":     "abc",
				"path":      "/",
				"http_only": true,
				"secure":    false,
				"same_site": "Lax",
			},
		},
		"body": map[string]any{"logged_in": true},
	}, loginNormalized)
	assert.Equal(t, map[string]any{
		"status": http.StatusOK,
		"body": map[string]any{
			"session": "abc",
		},
	}, meNormalized)
}

// TestCallHandlerRejectsCookieReuseWithoutJar verifies that cookie reuse is explicit per harness.
func TestCallHandlerRejectsCookieReuseWithoutJar(t *testing.T) {
	t.Parallel()

	// ARRANGE: the harness has an HTTP handler but no cookie jar for cross-step cookie state.
	harness := &testHarness{
		handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusNoContent)
		}),
		jar: nil,
	}
	callHandler := NewCallHandler[*testHarness]()
	request, err := callHandler.DecodeRequest(json.RawMessage(`{
		"method": "GET",
		"path": "/me",
		"use_cookies": true
	}`))
	require.NoError(t, err)

	// ACT: invoking with use_cookies requires the harness to expose a CookieJar.
	_, err = callHandler.Invoke(context.Background(), harness, request)

	// ASSERT: the error explains the missing per-case cookie state.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTPCookieJar")
}

// TestCallHandlerRejectsAmbiguousBody verifies that structured and raw request bodies are exclusive.
func TestCallHandlerRejectsAmbiguousBody(t *testing.T) {
	t.Parallel()

	// ARRANGE: the fixture contains two body sources, which would make request bytes ambiguous.
	callHandler := NewCallHandler[*testHarness]()
	request, err := callHandler.DecodeRequest(json.RawMessage(`{
		"method": "POST",
		"path": "/bad",
		"body": {"ok": true},
		"raw_body": "not-json"
	}`))
	require.NoError(t, err)

	// ACT: request construction rejects the ambiguous body before calling the HTTP handler.
	_, err = callHandler.Invoke(t.Context(), &testHarness{handler: http.NewServeMux(), jar: nil}, request)

	// ASSERT: the error points to the mutually exclusive fields.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "body and raw_body are mutually exclusive")
}

// TestCallHandlerNormalizesTextBody verifies that non-JSON responses remain assertable as strings.
func TestCallHandlerNormalizesTextBody(t *testing.T) {
	t.Parallel()

	// ARRANGE: the handler returns a non-JSON response body.
	harness := &testHarness{
		handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "text/plain")
			_, err := writer.Write([]byte(" plain response \n"))
			assert.NoError(t, err)
		}),
		jar: nil,
	}
	callHandler := NewCallHandler[*testHarness]()
	request, err := callHandler.DecodeRequest(json.RawMessage(`{"method":"GET","path":"/plain"}`))
	require.NoError(t, err)

	// ACT: normalization keeps trimmed text because there is no JSON object to decode.
	response, err := callHandler.Invoke(t.Context(), harness, request)
	require.NoError(t, err)
	normalized, err := callHandler.NormalizeResponse(response)
	require.NoError(t, err)

	// ASSERT: the text body can be compared in JSONC fixtures through assert.response.body.
	assert.Equal(t, map[string]any{
		"status": http.StatusOK,
		"body":   strings.TrimSpace(" plain response \n"),
	}, normalized)
}
