package httpserverexample

import (
	"github.com/n-r-w/itestkit"
	itestkithttpserver "github.com/n-r-w/itestkit/httpserver"
)

// newRegistry registers the preset CallHTTP handler for inbound HTTP API calls.
func newRegistry() itestkit.MapRegistry[*harness] {
	return itestkithttpserver.NewRegistry[*harness](
		nil,
		itestkithttpserver.WithBaseURL("http://api.example.test"),
	)
}
