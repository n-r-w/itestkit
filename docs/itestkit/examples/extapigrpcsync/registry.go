package extapigrpcsync

import (
	"github.com/n-r-w/itestkit"
)

// newGRPCSyncRegistry builds handlers for prepare/action/verify steps.
//
// There is no transport-specific runner logic here; all domain logic stays in handlers/harness.
func newGRPCSyncRegistry() itestkit.MapRegistry[*grpcSyncHarness] {
	return itestkit.NewMapRegistry(map[string]itestkit.Handler[*grpcSyncHarness]{
		"PlanExternalCheck":   planExternalCheckHandler{},
		"CallAPI":             callAPIHandler{},
		"VerifyExternalCheck": verifyExternalCheckHandler{},
	})
}
