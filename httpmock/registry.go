package httpmock

import (
	"maps"

	"github.com/n-r-w/itestkit"
)

// NewRegistry returns a registry with preset HTTP mock handlers and custom handlers.
//
// The case harness type C must expose HTTPMock() *Server.
func NewRegistry[C serverProvider](customHandlers map[string]itestkit.Handler[C]) itestkit.MapRegistry[C] {
	handlers := map[string]itestkit.Handler[C]{
		PlanHTTPCallsHandlerName:   PlanHTTPCallsHandler[C]{},
		AwaitHTTPCallsHandlerName:  AwaitHTTPCallsHandler[C]{},
		VerifyHTTPCallsHandlerName: VerifyHTTPCallsHandler[C]{},
	}
	maps.Copy(handlers, customHandlers)

	return itestkit.NewMapRegistry(handlers)
}
