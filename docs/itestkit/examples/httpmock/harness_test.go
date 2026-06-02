package httpmockexample

import (
	"testing"

	itestkithttpmock "github.com/n-r-w/itestkit/httpmock"
)

type harness struct {
	server *itestkithttpmock.Server
	client *orderClient
}

// HTTPMock returns the per-case HTTP mock server used by preset handlers.
func (testHarness *harness) HTTPMock() *itestkithttpmock.Server {
	return testHarness.server
}

type harnessFactory struct{}

// SetupSuite returns empty suite state because this example has no shared resources.
func (harnessFactory) SetupSuite(_ *testing.T) (struct{}, error) {
	return struct{}{}, nil
}

// TeardownSuite releases no resources because the HTTP mock server is per case.
func (harnessFactory) TeardownSuite(_ *testing.T, _ struct{}) error {
	return nil
}

// NewCaseHarness creates isolated HTTP mock state for one JSONC case.
func (harnessFactory) NewCaseHarness(t *testing.T, _ struct{}) *harness {
	t.Helper()

	server := itestkithttpmock.NewServer(t)
	return &harness{
		server: server,
		client: newOrderClient(server.URL()),
	}
}
