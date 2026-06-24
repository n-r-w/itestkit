package httpmock

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

const (
	defaultMaxRequestBodyBytes = int64(1024 * 1024)
	unexpectedRequestBody      = "unexpected httpmock request"
	requestBodyTooLargeBody    = "httpmock request body is too large"
)

// Option changes server construction settings.
type Option func(*config)

// Server records HTTP requests and returns planned responses.
type Server struct {
	mu                  sync.Mutex
	httpServer          *httptest.Server
	plan                Plan
	observed            []ObservedRequest
	maxRequestBodyBytes int64
	passThrough         http.Handler
}

var _ http.Handler = (*Server)(nil)

type config struct {
	maxRequestBodyBytes int64
}

// WithMaxRequestBodyBytes sets the maximum recorded request body size.
func WithMaxRequestBodyBytes(size int64) Option {
	return func(cfg *config) {
		if size > 0 {
			cfg.maxRequestBodyBytes = size
		}
	}
}

// NewServer starts an HTTP mock server for one test case.
func NewServer(t *testing.T, opts ...Option) *Server {
	t.Helper()

	cfg := config{maxRequestBodyBytes: defaultMaxRequestBodyBytes}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	server := &Server{
		mu:                  sync.Mutex{},
		httpServer:          nil,
		plan:                Plan{Calls: nil, Ordering: ""},
		observed:            nil,
		maxRequestBodyBytes: cfg.maxRequestBodyBytes,
		passThrough:         nil,
	}
	cleanupOnce := sync.Once{}
	cleanup := func() {
		cleanupOnce.Do(func() {
			if server.httpServer != nil {
				server.httpServer.Close()
			}
		})
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			cleanup()
			panic(recovered)
		}
	}()

	server.httpServer = httptest.NewServer(server)
	t.Cleanup(cleanup)

	return server
}

// NewPassThroughServer starts an HTTP mock server that verifies requests and delegates matched calls to handler.
func NewPassThroughServer(t *testing.T, handler http.Handler, opts ...Option) *Server {
	t.Helper()

	server := NewServer(t, opts...)
	server.passThrough = handler
	return server
}

// URL returns the mock server base URL.
func (server *Server) URL() string {
	return server.httpServer.URL
}

// Plan stores planned HTTP calls.
func (server *Server) Plan(ctx context.Context, plan Plan) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("plan http calls: %w", err)
	}

	normalized, err := normalizePlan(plan)
	if err != nil {
		return err
	}

	server.mu.Lock()
	defer server.mu.Unlock()

	server.plan = normalized
	server.observed = server.observed[:0]

	return nil
}

// Await checks whether planned HTTP calls have happened yet.
func (server *Server) Await(_ context.Context) (CheckResult, error) {
	server.mu.Lock()
	plan := clonePlan(server.plan)
	observed := cloneObservedRequests(server.observed)
	server.mu.Unlock()

	return evaluate(plan, observed, false)
}

// Verify checks final HTTP call expectations.
func (server *Server) Verify(ctx context.Context) (CheckResult, error) {
	if err := ctx.Err(); err != nil {
		return CheckResult{}, fmt.Errorf("verify http calls: %w", err)
	}

	server.mu.Lock()
	plan := clonePlan(server.plan)
	observed := cloneObservedRequests(server.observed)
	server.mu.Unlock()

	return evaluate(plan, observed, true)
}

// ServeHTTP records HTTP requests and returns planned responses.
func (server *Server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	observed, err := server.readObservedRequest(writer, request)
	if err != nil {
		http.Error(writer, requestBodyTooLargeBody, http.StatusRequestEntityTooLarge)
		return
	}

	server.mu.Lock()
	server.observed = append(server.observed, observed)
	plan := clonePlan(server.plan)
	observedSnapshot := cloneObservedRequests(server.observed)
	server.mu.Unlock()

	response, matched := responseForRequest(plan, observedSnapshot, observed)
	if !matched {
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = writer.Write([]byte(unexpectedRequestBody))
		return
	}

	if server.passThrough != nil {
		request.Body = io.NopCloser(bytes.NewReader([]byte(observed.Body)))
		server.passThrough.ServeHTTP(writer, request)
		return
	}

	writePlannedResponse(writer, response)
}

func (server *Server) readObservedRequest(
	_ http.ResponseWriter,
	request *http.Request,
) (ObservedRequest, error) {
	body, err := readBoundedBody(request.Body, server.maxRequestBodyBytes)
	if err != nil {
		return ObservedRequest{}, err
	}

	return ObservedRequest{
		Method:  request.Method,
		Path:    request.URL.Path,
		Query:   cloneValues(request.URL.Query()),
		Headers: cloneHeader(request.Header),
		Body:    string(body),
	}, nil
}

func readBoundedBody(reader io.ReadCloser, limit int64) ([]byte, error) {
	if reader == nil {
		return nil, nil
	}
	defer func() {
		_ = reader.Close()
	}()

	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, errors.New("request body exceeds limit")
	}

	return body, nil
}

func writePlannedResponse(writer http.ResponseWriter, response Response) {
	for name, values := range response.Headers {
		for _, value := range values {
			writer.Header().Add(name, value)
		}
	}
	writer.WriteHeader(response.Status)

	body := responseBodyBytes(response)
	if len(body) == 0 {
		return
	}
	_, _ = io.Copy(writer, bytes.NewReader(body))
}
