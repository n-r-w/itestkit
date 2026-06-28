package httpserver

import (
	"maps"

	"github.com/n-r-w/itestkit"
)

const (
	// CallHTTPHandlerName is the preset handler name for in-process HTTP calls.
	CallHTTPHandlerName = "CallHTTP"
)

// NewRegistry returns a registry with the preset HTTP server handler and custom handlers.
//
// The case harness type C must expose HTTPHandler() http.Handler. If fixtures use use_cookies,
// the same harness must also expose HTTPCookieJar() *httpserver.CookieJar.
func NewRegistry[C handlerProvider](
	customHandlers map[string]itestkit.Handler[C],
	options ...Option,
) itestkit.MapRegistry[C] {
	handlers := map[string]itestkit.Handler[C]{
		CallHTTPHandlerName: NewCallHandler[C](options...),
	}
	maps.Copy(handlers, customHandlers)

	return itestkit.NewMapRegistry(handlers)
}
