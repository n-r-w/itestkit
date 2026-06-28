// Package httpserver provides itestkit handlers for in-process HTTP servers.
package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"

	"github.com/n-r-w/itestkit"
)

const (
	defaultBaseURL = "http://itestkit.local"

	bodyKey     = "body"
	cookiesKey  = "cookies"
	headersKey  = "headers"
	httpOnlyKey = "http_only"
	maxAgeKey   = "max_age"
	pathKey     = "path"
	sameSiteKey = "same_site"
	secureKey   = "secure"
	statusKey   = "status"
	valueKey    = "value"

	sameSiteDefault = "Default"
	sameSiteLax     = "Lax"
	sameSiteStrict  = "Strict"
	sameSiteNone    = "None"
)

// handlerProvider exposes the HTTP handler used by CallHandler.
type handlerProvider interface {
	HTTPHandler() http.Handler
}

// cookieJarProvider exposes per-case HTTP cookie state.
type cookieJarProvider interface {
	HTTPCookieJar() *CookieJar
}

// Request describes one in-process HTTP request from a JSONC step.
type Request struct {
	Method         string            `json:"method"`
	Path           string            `json:"path"`
	Query          map[string]string `json:"query,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	Body           json.RawMessage   `json:"body,omitempty"`
	RawBody        *string           `json:"raw_body,omitempty"`
	UseCookies     bool              `json:"use_cookies,omitempty"`
	CSRF           *CSRFConfig       `json:"csrf,omitempty"`
	CaptureHeaders []string          `json:"capture_headers,omitempty"`
	CaptureCookies []string          `json:"capture_cookies,omitempty"`
}

// CSRFConfig copies one stored cookie value into one request header.
type CSRFConfig struct {
	// Cookie names the cookie in the per-case CookieJar whose value becomes the CSRF token.
	Cookie string `json:"cookie"`
	// Header names the HTTP header that receives the cookie value.
	Header string `json:"header"`
}

// Response is the stable JSON-safe HTTP response shape asserted by fixtures.
type Response struct {
	Status  int                 `json:"status"`
	Headers map[string][]string `json:"headers,omitempty"`
	Cookies map[string]Cookie   `json:"cookies,omitempty"`
	Body    any                 `json:"body,omitempty"`
}

// Cookie is a stable JSON-safe view of a Set-Cookie value.
type Cookie struct {
	Value    string `json:"value"`
	Path     string `json:"path"`
	HTTPOnly bool   `json:"http_only"`
	Secure   bool   `json:"secure"`
	SameSite string `json:"same_site"`
	MaxAge   int    `json:"max_age,omitempty"`
}

// CookieJar stores HTTP cookies within one test case.
type CookieJar struct {
	cookies map[string]*http.Cookie
}

// NewCookieJar creates an empty cookie jar for one test case.
func NewCookieJar() *CookieJar {
	return &CookieJar{cookies: make(map[string]*http.Cookie)}
}

// Option configures CallHandler.
type Option func(*callHandlerConfig)

// callHandlerConfig stores CallHandler options.
type callHandlerConfig struct {
	baseURL string
}

// WithBaseURL sets the base URL used to construct in-process HTTP requests.
//
// Host-aware routers read request.Host from this URL. The request path and query still come from JSONC fields.
func WithBaseURL(rawURL string) Option {
	return func(config *callHandlerConfig) {
		config.baseURL = strings.TrimSpace(rawURL)
	}
}

// CallHandler executes JSONC HTTP requests against an in-process HTTP handler.
type CallHandler[C handlerProvider] struct {
	config callHandlerConfig
}

// NewCallHandler creates a CallHandler.
func NewCallHandler[C handlerProvider](options ...Option) CallHandler[C] {
	config := callHandlerConfig{baseURL: defaultBaseURL}
	for _, option := range options {
		option(&config)
	}
	if config.baseURL == "" {
		config.baseURL = defaultBaseURL
	}

	return CallHandler[C]{config: config}
}

var _ itestkit.Handler[handlerProvider] = CallHandler[handlerProvider]{}

// DecodeRequest decodes a JSONC HTTP request with strict field validation.
func (CallHandler[C]) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &Request{}
	if err := itestkit.DecodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode HTTP request: %w", err)
	}

	return request, nil
}

// DecodeExpectedResponse decodes the expected normalized HTTP response.
func (CallHandler[C]) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &Response{}
	if err := itestkit.DecodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode HTTP response: %w", err)
	}

	return response, nil
}

// Invoke sends the fixture request to the in-process HTTP handler.
func (handler CallHandler[C]) Invoke(ctx context.Context, harness C, request any) (any, error) {
	typedRequest, ok := request.(*Request)
	if !ok {
		return nil, fmt.Errorf("CallHTTP received invalid request type: %T", request)
	}

	httpRequest, err := buildHTTPRequest(ctx, handler.config.baseURL, typedRequest)
	if err != nil {
		return nil, err
	}

	jar := resolveCookieJar(harness)
	if typedRequest.UseCookies {
		if jar == nil {
			return nil, errors.New("use_cookies requires harness method HTTPCookieJar() *httpserver.CookieJar")
		}
		jar.attach(httpRequest)
	}
	csrfErr := applyCSRFHeader(httpRequest, typedRequest, jar)
	if csrfErr != nil {
		return nil, csrfErr
	}

	return executeHTTPRequest(
		harness.HTTPHandler(),
		jar,
		httpRequest,
		typedRequest.CaptureHeaders,
		typedRequest.CaptureCookies,
	)
}

// NormalizeResponse converts handler responses to a stable map for exact and partial comparisons.
func (CallHandler[C]) NormalizeResponse(response any) (any, error) {
	typedResponse, ok := response.(*Response)
	if !ok {
		valueResponse, valueOK := response.(Response)
		if !valueOK {
			return nil, fmt.Errorf("CallHTTP received invalid response type: %T", response)
		}
		typedResponse = &valueResponse
	}

	normalized := map[string]any{statusKey: typedResponse.Status}
	if len(typedResponse.Headers) > 0 {
		normalized[headersKey] = typedResponse.Headers
	}
	if len(typedResponse.Cookies) > 0 {
		normalized[cookiesKey] = normalizeCookiesForResponse(typedResponse.Cookies)
	}
	if typedResponse.Body != nil {
		normalized[bodyKey] = typedResponse.Body
	}

	return normalized, nil
}

// buildHTTPRequest turns the JSONC request model into an HTTP request without hidden request defaults.
func buildHTTPRequest(ctx context.Context, baseURL string, request *Request) (*http.Request, error) {
	method := strings.TrimSpace(request.Method)
	if method == "" {
		return nil, errors.New("method is required")
	}
	if strings.ContainsAny(method, " \t\r\n") {
		return nil, fmt.Errorf("method must be an HTTP token without whitespace: %q", request.Method)
	}

	path := strings.TrimSpace(request.Path)
	if path == "" {
		return nil, errors.New("path is required")
	}
	if !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("path must start with /: %q", request.Path)
	}
	if strings.Contains(path, "?") {
		return nil, errors.New("path must not contain query string; use query field")
	}

	requestBody, err := buildRequestBody(request)
	if err != nil {
		return nil, err
	}

	requestURL, err := buildRequestURL(baseURL, path, request.Query)
	if err != nil {
		return nil, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, method, requestURL, requestBody)
	if err != nil {
		return nil, fmt.Errorf("build HTTP request: %w", err)
	}
	for name, value := range request.Headers {
		if strings.EqualFold(name, "Host") {
			httpRequest.Host = value
			continue
		}
		httpRequest.Header.Set(name, value)
	}

	return httpRequest, nil
}

// buildRequestBody returns either structured JSON fixture body or raw bytes for malformed-request cases.
func buildRequestBody(request *Request) (*bytes.Reader, error) {
	if len(request.Body) > 0 && request.RawBody != nil {
		return nil, errors.New("body and raw_body are mutually exclusive")
	}
	if request.RawBody != nil {
		return bytes.NewReader([]byte(*request.RawBody)), nil
	}
	return bytes.NewReader(request.Body), nil
}

// buildRequestURL combines base URL, path, and query fields into the URL used by the handler.
func buildRequestURL(baseURL, path string, query map[string]string) (string, error) {
	parsedBaseURL, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse base URL: %w", err)
	}
	if parsedBaseURL.Scheme == "" || parsedBaseURL.Host == "" {
		return "", fmt.Errorf("base URL must include scheme and host: %q", baseURL)
	}

	queryValues := make(url.Values, len(query))
	for name, value := range query {
		queryValues.Set(name, value)
	}

	requestURL := *parsedBaseURL
	requestURL.Path = path
	requestURL.RawQuery = queryValues.Encode()
	requestURL.Fragment = ""
	requestURL.RawFragment = ""
	return requestURL.String(), nil
}

// applyCSRFHeader copies a fixture-selected cookie token into a header without overriding manual headers.
func applyCSRFHeader(httpRequest *http.Request, request *Request, jar *CookieJar) error {
	if request.CSRF == nil {
		return nil
	}

	cookieName := strings.TrimSpace(request.CSRF.Cookie)
	if cookieName == "" {
		return errors.New("csrf.cookie is required")
	}
	headerName := strings.TrimSpace(request.CSRF.Header)
	if headerName == "" {
		return errors.New("csrf.header is required")
	}
	if hasManualHeader(request.Headers, headerName) {
		return fmt.Errorf("csrf header %q is already set in request headers", headerName)
	}
	if jar == nil {
		return errors.New("csrf requires harness method HTTPCookieJar() *httpserver.CookieJar")
	}

	cookieValue, exists := jar.cookieValue(cookieName)
	if !exists {
		return fmt.Errorf("csrf cookie %q is not available in HTTPCookieJar", cookieName)
	}
	httpRequest.Header.Set(headerName, cookieValue)
	return nil
}

// hasManualHeader detects fixture-declared headers case-insensitively to avoid hiding mismatch cases.
func hasManualHeader(headers map[string]string, headerName string) bool {
	for name := range headers {
		if strings.EqualFold(strings.TrimSpace(name), headerName) {
			return true
		}
	}
	return false
}

// executeHTTPRequest invokes the HTTP handler and captures only response data controlled by the fixture contract.
func executeHTTPRequest(
	httpHandler http.Handler,
	jar *CookieJar,
	request *http.Request,
	captureHeaders []string,
	captureCookies []string,
) (*Response, error) {
	if httpHandler == nil {
		return nil, errors.New("HTTPHandler returned nil")
	}

	responseRecorder := httptest.NewRecorder()
	httpHandler.ServeHTTP(responseRecorder, request)
	response := responseRecorder.Result()
	if jar != nil {
		jar.store(response.Cookies())
	}

	body, err := decodeHTTPResponseBody(responseRecorder)
	if err != nil {
		return nil, err
	}
	closeErr := response.Body.Close()
	if closeErr != nil {
		return nil, fmt.Errorf("close HTTP response body: %w", closeErr)
	}

	httpResponse := &Response{
		Status:  responseRecorder.Code,
		Headers: captureHTTPHeaders(responseRecorder, captureHeaders),
		Cookies: captureHTTPCookies(response.Cookies(), captureCookies),
		Body:    nil,
	}
	if body.present {
		httpResponse.Body = body.value
	}

	return httpResponse, nil
}

// decodedBody distinguishes an empty body from a body whose decoded JSON value is null.
type decodedBody struct {
	value   any
	present bool
}

// decodeHTTPResponseBody returns JSON bodies as JSON-safe data and non-JSON bodies as strings.
func decodeHTTPResponseBody(responseRecorder *httptest.ResponseRecorder) (decodedBody, error) {
	rawBody := bytes.TrimSpace(responseRecorder.Body.Bytes())
	if len(rawBody) == 0 {
		return decodedBody{value: nil, present: false}, nil
	}

	contentType := responseRecorder.Header().Get("Content-Type")
	if !strings.Contains(strings.ToLower(contentType), "json") {
		return decodedBody{value: string(rawBody), present: true}, nil
	}

	var body any
	decoder := json.NewDecoder(bytes.NewReader(rawBody))
	if err := decoder.Decode(&body); err != nil {
		return decodedBody{}, fmt.Errorf("decode JSON HTTP response: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return decodedBody{}, errors.New("decode JSON HTTP response: trailing data")
	}

	return decodedBody{value: body, present: true}, nil
}

// captureHTTPHeaders returns selected response headers requested by the JSONC fixture.
func captureHTTPHeaders(responseRecorder *httptest.ResponseRecorder, captureHeaders []string) map[string][]string {
	if len(captureHeaders) == 0 {
		return nil
	}

	headers := make(map[string][]string, len(captureHeaders))
	for _, headerName := range captureHeaders {
		trimmedName := strings.TrimSpace(headerName)
		if trimmedName == "" {
			continue
		}
		headers[trimmedName] = responseRecorder.Header().Values(trimmedName)
	}

	return headers
}

// captureHTTPCookies returns only the cookies requested by the JSONC fixture.
func captureHTTPCookies(cookies []*http.Cookie, captureCookies []string) map[string]Cookie {
	if len(captureCookies) == 0 {
		return nil
	}

	requested := make(map[string]struct{}, len(captureCookies))
	for _, name := range captureCookies {
		trimmedName := strings.TrimSpace(name)
		if trimmedName != "" {
			requested[trimmedName] = struct{}{}
		}
	}
	if len(requested) == 0 {
		return nil
	}

	captured := make(map[string]Cookie, len(requested))
	for _, cookie := range cookies {
		if _, ok := requested[cookie.Name]; !ok {
			continue
		}
		captured[cookie.Name] = normalizeHTTPCookie(cookie)
	}
	return captured
}

// normalizeCookiesForResponse converts cookie structs into maps so itestkit markers can target object fields.
func normalizeCookiesForResponse(cookies map[string]Cookie) map[string]map[string]any {
	normalized := make(map[string]map[string]any, len(cookies))
	for name, cookie := range cookies {
		cookieMap := map[string]any{
			valueKey:    cookie.Value,
			pathKey:     cookie.Path,
			httpOnlyKey: cookie.HTTPOnly,
			secureKey:   cookie.Secure,
			sameSiteKey: cookie.SameSite,
		}
		if cookie.MaxAge != 0 {
			cookieMap[maxAgeKey] = cookie.MaxAge
		}
		normalized[name] = cookieMap
	}
	return normalized
}

// normalizeHTTPCookie converts net/http cookies into the fixture assertion shape.
func normalizeHTTPCookie(cookie *http.Cookie) Cookie {
	return Cookie{
		Value:    cookie.Value,
		Path:     cookie.Path,
		HTTPOnly: cookie.HttpOnly,
		Secure:   cookie.Secure,
		SameSite: sameSiteString(cookie.SameSite),
		MaxAge:   cookie.MaxAge,
	}
}

// sameSiteString converts SameSite constants into stable fixture strings.
func sameSiteString(sameSite http.SameSite) string {
	switch sameSite {
	case http.SameSiteDefaultMode:
		return sameSiteDefault
	case http.SameSiteLaxMode:
		return sameSiteLax
	case http.SameSiteStrictMode:
		return sameSiteStrict
	case http.SameSiteNoneMode:
		return sameSiteNone
	default:
		return sameSiteDefault
	}
}

// resolveCookieJar returns optional per-case cookie state from the harness.
func resolveCookieJar(harness any) *CookieJar {
	provider, ok := harness.(cookieJarProvider)
	if !ok {
		return nil
	}

	return provider.HTTPCookieJar()
}

// attach adds stored cookies to the next HTTP request in stable order.
func (jar *CookieJar) attach(request *http.Request) {
	cookieNames := make([]string, 0, len(jar.cookies))
	for name := range jar.cookies {
		cookieNames = append(cookieNames, name)
	}
	sort.Strings(cookieNames)

	for _, name := range cookieNames {
		request.AddCookie(jar.cookies[name])
	}
}

// cookieValue returns a stored cookie value for fixture-controlled header derivation.
func (jar *CookieJar) cookieValue(name string) (string, bool) {
	if jar.cookies == nil {
		return "", false
	}
	cookie, exists := jar.cookies[name]
	if !exists {
		return "", false
	}
	return cookie.Value, true
}

// store records Set-Cookie values for later explicit use by JSONC requests.
func (jar *CookieJar) store(cookies []*http.Cookie) {
	if jar.cookies == nil {
		jar.cookies = make(map[string]*http.Cookie)
	}
	for _, cookie := range cookies {
		if cookie.MaxAge < 0 {
			delete(jar.cookies, cookie.Name)
			continue
		}
		cookieCopy := *cookie
		jar.cookies[cookie.Name] = &cookieCopy
	}
}
