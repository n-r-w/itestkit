package kafkaoutbound

import (
	"github.com/n-r-w/itestkit"
	"github.com/n-r-w/itestkit/queue/itest"
)

// newRegistry combines preset outbound handlers with domain action handler.
func newRegistry() itestkit.MapRegistry[*outboundHarness] {
	return itest.NewRegistry(map[string]itestkit.Handler[*outboundHarness]{
		"SendOrderEvent": sendOrderEventHandler{},
	})
}
