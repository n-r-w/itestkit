package itests

import (
	"github.com/n-r-w/itestkit"
)

// newHandlerRegistry creates the full handler set for the real example.
func newHandlerRegistry() itestkit.MapRegistry[*caseHarness] {
	return itestkit.NewMapRegistry(map[string]itestkit.Handler[*caseHarness]{
		"SeedOrders":          seedOrdersHandler{},
		"PlanExternalCalls":   planExternalCallsHandler{},
		"CallOrderAPI":        callOrderAPIHandler{},
		"VerifyExternalCalls": verifyExternalCallsHandler{},
		"VerifyOrderState":    verifyOrderStateHandler{},
	})
}
