package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"
)

// generatedHarness combines generated mocks required by CallHandler tests.
type generatedHarness struct {
	*MockhandlerProvider
	*MockcookieJarProvider
}

// newGeneratedHarness creates a generated mock harness for one test case.
func newGeneratedHarness(t *testing.T, handler http.Handler, jar *CookieJar) *generatedHarness {
	ctrl := gomock.NewController(t)
	harness := &generatedHarness{
		MockhandlerProvider:   NewMockhandlerProvider(ctrl),
		MockcookieJarProvider: NewMockcookieJarProvider(ctrl),
	}
	harness.MockhandlerProvider.EXPECT().HTTPHandler().Return(handler).AnyTimes()
	harness.MockcookieJarProvider.EXPECT().HTTPCookieJar().Return(jar).AnyTimes()
	return harness
}

// TestCallHandlerInvokesHTTPHandler verifies request construction and normalized JSON response output.
func TestCallHandlerInvokesHTTPHandler(t *testing.T) {
	t.Parallel()

	// ARRANGE: the HTTP handler checks the transport-level request created from JSONC fields.
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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
	harness := newGeneratedHarness(t, handler, nil)

	callHandler := NewCallHandler[*generatedHarness](WithBaseURL("http://api.test"))
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

// TestCallHandlerCapturesAbsentHeaderAsEmptyList verifies that requested missing headers stay assertable as arrays.
func TestCallHandlerCapturesAbsentHeaderAsEmptyList(t *testing.T) {
	t.Parallel()

	// ARRANGE: the handler returns JSON without the requested Set-Cookie header.
	handler := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, err := writer.Write([]byte(`{"ok":true}`))
		assert.NoError(t, err)
	})
	harness := newGeneratedHarness(t, handler, nil)
	callHandler := NewCallHandler[*generatedHarness]()
	request, err := callHandler.DecodeRequest(json.RawMessage(`{
		"method": "GET",
		"path": "/without-cookie",
		"capture_headers": ["Set-Cookie"]
	}`))
	require.NoError(t, err)

	// ACT: response normalization includes the requested header even though it is absent.
	response, err := callHandler.Invoke(t.Context(), harness, request)
	require.NoError(t, err)
	normalized, err := callHandler.NormalizeResponse(response)
	require.NoError(t, err)

	// ASSERT: exact comparisons can distinguish an explicitly captured absent header from an omitted header.
	assert.Equal(t, map[string]any{
		"status": http.StatusOK,
		"headers": map[string][]string{
			"Set-Cookie": {},
		},
		"body": map[string]any{"ok": true},
	}, normalized)
}

// TestCallHandlerKeepsCookiesInCaseJar verifies explicit cookie reuse between case steps.
func TestCallHandlerKeepsCookiesInCaseJar(t *testing.T) {
	t.Parallel()

	// ARRANGE: one endpoint issues a session cookie and another endpoint requires it.
	jar := NewCookieJar()
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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
	harness := newGeneratedHarness(t, handler, jar)

	callHandler := NewCallHandler[*generatedHarness]()

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

// TestCallHandlerSetsCSRFHeaderFromCookieJar verifies explicit CSRF header creation from stored cookies.
func TestCallHandlerSetsCSRFHeaderFromCookieJar(t *testing.T) {
	t.Parallel()

	// ARRANGE: the fixture asks CallHTTP to reuse cookies and copy one cookie value into a CSRF header.
	jar := NewCookieJar()
	jar.store([]*http.Cookie{{Name: "csrf_token", Value: "token-1"}})
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		cookie, err := request.Cookie("csrf_token")
		if !assert.NoError(t, err) {
			return
		}
		assert.Equal(t, "token-1", cookie.Value)
		assert.Equal(t, "token-1", request.Header.Get("X-CSRF-Token"))
		writer.Header().Set("Content-Type", "application/json")
		_, err = writer.Write([]byte(`{"ok":true}`))
		assert.NoError(t, err)
	})
	harness := newGeneratedHarness(t, handler, jar)
	callHandler := NewCallHandler[*generatedHarness]()
	request, err := callHandler.DecodeRequest(json.RawMessage(`{
		"method": "POST",
		"path": "/mutate",
		"use_cookies": true,
		"csrf": {"cookie": "csrf_token", "header": "X-CSRF-Token"}
	}`))
	require.NoError(t, err)

	// ACT: invoking the request copies the stored cookie value into the configured header.
	response, err := callHandler.Invoke(t.Context(), harness, request)
	require.NoError(t, err)
	normalized, err := callHandler.NormalizeResponse(response)
	require.NoError(t, err)

	// ASSERT: the request reached the handler and response normalization stayed unchanged.
	assert.Equal(t, map[string]any{
		"status": http.StatusOK,
		"body":   map[string]any{"ok": true},
	}, normalized)
}

// TestCallHandlerRejectsCSRFWithoutCookie verifies that fixtures fail when requested CSRF state is unavailable.
func TestCallHandlerRejectsCSRFWithoutCookie(t *testing.T) {
	t.Parallel()

	// ARRANGE: the fixture requests a CSRF header from an empty per-case cookie jar.
	handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler must not be called when CSRF cookie is missing")
	})
	harness := newGeneratedHarness(t, handler, NewCookieJar())
	callHandler := NewCallHandler[*generatedHarness]()
	request, err := callHandler.DecodeRequest(json.RawMessage(`{
		"method": "POST",
		"path": "/mutate",
		"csrf": {"cookie": "csrf_token", "header": "X-CSRF-Token"}
	}`))
	require.NoError(t, err)

	// ACT: CallHTTP rejects the fixture before invoking the HTTP handler.
	_, err = callHandler.Invoke(t.Context(), harness, request)

	// ASSERT: the error names the missing cookie so the fixture can be fixed directly.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "csrf_token")
}

// TestCallHandlerRejectsCSRFHeaderConflict verifies that manual mismatch requests stay explicit.
func TestCallHandlerRejectsCSRFHeaderConflict(t *testing.T) {
	t.Parallel()

	// ARRANGE: the fixture sets the CSRF header manually and also asks the helper to derive it.
	jar := NewCookieJar()
	jar.store([]*http.Cookie{{Name: "csrf_token", Value: "token-1"}})
	handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler must not be called when CSRF header is already set")
	})
	harness := newGeneratedHarness(t, handler, jar)
	callHandler := NewCallHandler[*generatedHarness]()
	request, err := callHandler.DecodeRequest(json.RawMessage(`{
		"method": "POST",
		"path": "/mutate",
		"headers": {"x-csrf-token": "manual-token"},
		"csrf": {"cookie": "csrf_token", "header": "X-CSRF-Token"}
	}`))
	require.NoError(t, err)

	// ACT: CallHTTP refuses to overwrite the manually declared header value.
	_, err = callHandler.Invoke(t.Context(), harness, request)

	// ASSERT: the error keeps mismatch tests visible in JSONC fixtures.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "X-CSRF-Token")
}

// TestCallHandlerRejectsCookieReuseWithoutJar verifies that cookie reuse is explicit per harness.
func TestCallHandlerRejectsCookieReuseWithoutJar(t *testing.T) {
	t.Parallel()

	// ARRANGE: the harness has an HTTP handler but no cookie jar for cross-step cookie state.
	handler := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	})
	harness := newGeneratedHarness(t, handler, nil)
	callHandler := NewCallHandler[*generatedHarness]()
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
	callHandler := NewCallHandler[*generatedHarness]()
	request, err := callHandler.DecodeRequest(json.RawMessage(`{
		"method": "POST",
		"path": "/bad",
		"body": {"ok": true},
		"raw_body": "not-json"
	}`))
	require.NoError(t, err)

	// ACT: request construction rejects the ambiguous body before calling the HTTP handler.
	harness := newGeneratedHarness(t, http.NewServeMux(), nil)
	_, err = callHandler.Invoke(t.Context(), harness, request)

	// ASSERT: the error points to the mutually exclusive fields.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "body and raw_body are mutually exclusive")
}

// TestCallHandlerNormalizesTextBody verifies that non-JSON responses remain assertable as strings.
func TestCallHandlerNormalizesTextBody(t *testing.T) {
	t.Parallel()

	// ARRANGE: the handler returns a non-JSON response body.
	handler := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/plain")
		_, err := writer.Write([]byte(" plain response \n"))
		assert.NoError(t, err)
	})
	harness := newGeneratedHarness(t, handler, nil)
	callHandler := NewCallHandler[*generatedHarness]()
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

// TestCallHandlerBodyNormalizerHandlesRawBinaryBody verifies custom JSON-safe summaries for binary responses.
func TestCallHandlerBodyNormalizerHandlesRawBinaryBody(t *testing.T) {
	t.Parallel()

	// ARRANGE: the fixture declares response_body settings and the handler returns bytes that are not useful as text.
	binaryBody := []byte{0x00, 'x', 0xff, '\n'}
	handler := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		_, err := writer.Write(binaryBody)
		assert.NoError(t, err)
	})
	harness := newGeneratedHarness(t, handler, nil)
	normalizer := NewMockBodyNormalizer(gomock.NewController(t))
	normalizer.EXPECT().
		NormalizeHTTPBody(gomock.AssignableToTypeOf(BodyNormalizationContext{})).
		DoAndReturn(func(ctx BodyNormalizationContext) (any, bool, error) {
			assert.Equal(t, binaryBody, ctx.Body)
			assert.Equal(t, http.StatusOK, ctx.Response.StatusCode)
			assert.Equal(t, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ctx.Response.Header.Get("Content-Type"))

			var config struct {
				Type  string `json:"type"`
				Sheet string `json:"sheet"`
			}
			require.NoError(t, json.Unmarshal(ctx.Request.ResponseBody, &config))

			return map[string]any{
				"type":   config.Type,
				"sheet":  config.Sheet,
				"length": len(ctx.Body),
			}, true, nil
		})
	callHandler := NewCallHandler[*generatedHarness](WithBodyNormalizer(normalizer))
	request, err := callHandler.DecodeRequest(json.RawMessage(`{
		"method": "GET",
		"path": "/file.xlsx",
		"response_body": {"type": "xlsx", "sheet": "Report"}
	}`))
	require.NoError(t, err)

	// ACT: the normalizer replaces the default string conversion with a structured summary.
	response, err := callHandler.Invoke(t.Context(), harness, request)
	require.NoError(t, err)
	normalized, err := callHandler.NormalizeResponse(response)
	require.NoError(t, err)

	// ASSERT: the body is the JSON-safe object returned by the normalizer.
	assert.Equal(t, map[string]any{
		"status": http.StatusOK,
		"body": map[string]any{
			"type":   "xlsx",
			"sheet":  "Report",
			"length": len(binaryBody),
		},
	}, normalized)
}

// TestCallHandlerBodyNormalizerFallsBackWhenUnhandled verifies handled=false preserves default body decoding.
func TestCallHandlerBodyNormalizerFallsBackWhenUnhandled(t *testing.T) {
	t.Parallel()

	// ARRANGE: the normalizer inspects raw bytes but declines ownership of a plain text response.
	handler := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/plain")
		_, err := writer.Write([]byte(" fallback text \n"))
		assert.NoError(t, err)
	})
	harness := newGeneratedHarness(t, handler, nil)
	normalizer := NewMockBodyNormalizer(gomock.NewController(t))
	normalizer.EXPECT().
		NormalizeHTTPBody(gomock.AssignableToTypeOf(BodyNormalizationContext{})).
		DoAndReturn(func(ctx BodyNormalizationContext) (any, bool, error) {
			assert.Equal(t, []byte(" fallback text \n"), ctx.Body)
			return nil, false, nil
		})
	callHandler := NewCallHandler[*generatedHarness](WithBodyNormalizer(normalizer))
	request, err := callHandler.DecodeRequest(json.RawMessage(`{"method":"GET","path":"/plain"}`))
	require.NoError(t, err)

	// ACT: unhandled bodies continue through the existing text fallback.
	response, err := callHandler.Invoke(t.Context(), harness, request)
	require.NoError(t, err)
	normalized, err := callHandler.NormalizeResponse(response)
	require.NoError(t, err)

	// ASSERT: the hook was offered the body, then default normalization produced trimmed text.
	assert.Equal(t, map[string]any{
		"status": http.StatusOK,
		"body":   "fallback text",
	}, normalized)
}

// TestCallHandlerBodyNormalizerErrorFailsInvoke verifies hook errors fail the CallHTTP step.
func TestCallHandlerBodyNormalizerErrorFailsInvoke(t *testing.T) {
	t.Parallel()

	// ARRANGE: the normalizer rejects the response body contract.
	expectedErr := errors.New("normalize body")
	handler := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, err := writer.Write([]byte("bad body"))
		assert.NoError(t, err)
	})
	harness := newGeneratedHarness(t, handler, nil)
	normalizer := NewMockBodyNormalizer(gomock.NewController(t))
	normalizer.EXPECT().
		NormalizeHTTPBody(gomock.AssignableToTypeOf(BodyNormalizationContext{})).
		Return(nil, false, expectedErr)
	callHandler := NewCallHandler[*generatedHarness](WithBodyNormalizer(normalizer))
	request, err := callHandler.DecodeRequest(json.RawMessage(`{"method":"GET","path":"/bad"}`))
	require.NoError(t, err)

	// ACT: Invoke returns the normalizer error before response normalization.
	_, err = callHandler.Invoke(t.Context(), harness, request)

	// ASSERT: the step fails with the normalizer error so fixtures do not compare invalid summaries.
	require.Error(t, err)
	assert.ErrorIs(t, err, expectedErr)
}

// TestCallHandlerBodyNormalizerKeepsCapturedHeadersAndCookies verifies body hooks do not own response metadata.
func TestCallHandlerBodyNormalizerKeepsCapturedHeadersAndCookies(t *testing.T) {
	t.Parallel()

	// ARRANGE: the response includes metadata that must stay controlled by capture options.
	handler := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/octet-stream")
		writer.Header().Set("X-Trace", "trace-2")
		cookie := new(http.Cookie)
		cookie.Name = "download_id"
		cookie.Value = "file-1"
		cookie.Path = "/"
		http.SetCookie(writer, cookie)
		_, err := writer.Write([]byte{0x01, 0x02})
		assert.NoError(t, err)
	})
	harness := newGeneratedHarness(t, handler, nil)
	normalizer := NewMockBodyNormalizer(gomock.NewController(t))
	normalizer.EXPECT().
		NormalizeHTTPBody(gomock.AssignableToTypeOf(BodyNormalizationContext{})).
		DoAndReturn(func(ctx BodyNormalizationContext) (any, bool, error) {
			ctx.Response.Header.Set("X-Trace", "mutated")
			return map[string]any{"bytes": len(ctx.Body)}, true, nil
		})
	callHandler := NewCallHandler[*generatedHarness](WithBodyNormalizer(normalizer))
	request, err := callHandler.DecodeRequest(json.RawMessage(`{
		"method": "GET",
		"path": "/file.bin",
		"capture_headers": ["X-Trace"],
		"capture_cookies": ["download_id"]
	}`))
	require.NoError(t, err)

	// ACT: body normalization runs in the same CallHTTP path as header and cookie capture.
	response, err := callHandler.Invoke(t.Context(), harness, request)
	require.NoError(t, err)
	normalized, err := callHandler.NormalizeResponse(response)
	require.NoError(t, err)

	// ASSERT: captured metadata keeps the original response values while the body uses the hook result.
	assert.Equal(t, map[string]any{
		"status": http.StatusOK,
		"headers": map[string][]string{
			"X-Trace": {"trace-2"},
		},
		"cookies": map[string]map[string]any{
			"download_id": {
				"value":     "file-1",
				"path":      "/",
				"http_only": false,
				"secure":    false,
				"same_site": "Default",
			},
		},
		"body": map[string]any{"bytes": 2},
	}, normalized)
}
