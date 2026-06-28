package httpserverexample

import (
	"net/http"
	"testing"

	itestkithttpserver "github.com/n-r-w/itestkit/httpserver"
)

// harness owns one isolated HTTP API instance and cookie jar for a JSONC case.
type harness struct {
	handler http.Handler
	cookies *itestkithttpserver.CookieJar
}

// HTTPHandler returns the in-process HTTP API called by the CallHTTP handler.
func (testHarness *harness) HTTPHandler() http.Handler {
	return testHarness.handler
}

// HTTPCookieJar returns per-case cookies used only when JSONC requests set use_cookies.
func (testHarness *harness) HTTPCookieJar() *itestkithttpserver.CookieJar {
	return testHarness.cookies
}

// harnessFactory creates suite and per-case test state.
type harnessFactory struct{}

// SetupSuite returns empty suite state because the example creates all state per case.
func (harnessFactory) SetupSuite(_ *testing.T) (struct{}, error) {
	return struct{}{}, nil
}

// TeardownSuite releases no suite resources because each case owns its HTTP handler and cookies.
func (harnessFactory) TeardownSuite(_ *testing.T, _ struct{}) error {
	return nil
}

// NewCaseHarness creates an isolated HTTP API and cookie jar for one JSONC case.
func (harnessFactory) NewCaseHarness(t *testing.T, _ struct{}) *harness {
	t.Helper()

	return &harness{
		handler: newAPIHandler(),
		cookies: itestkithttpserver.NewCookieJar(),
	}
}
