package itest

import (
	"maps"

	"github.com/n-r-w/itestkit"
)

// NewRegistry creates a MapRegistry with preset outbound handlers and custom action handlers.
func NewRegistry[C OutboundHarness](customHandlers map[string]itestkit.Handler[C]) itestkit.MapRegistry[C] {
	handlers := map[string]itestkit.Handler[C]{
		PlanOutboundHandlerName:   PlanOutboundHandler[C]{},
		AwaitOutboundHandlerName:  AwaitOutboundHandler[C]{},
		VerifyOutboundHandlerName: VerifyOutboundHandler[C]{},
		CleanupBrokerHandlerName:  CleanupBrokerHandler[C]{},
	}
	maps.Copy(handlers, customHandlers)

	return itestkit.NewMapRegistry(handlers)
}
