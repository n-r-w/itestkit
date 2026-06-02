package httpmockexample

import (
	"github.com/n-r-w/itestkit"
	itestkithttpmock "github.com/n-r-w/itestkit/httpmock"
)

// newRegistry combines HTTP mock preset handlers with the example action handler.
func newRegistry() itestkit.MapRegistry[*harness] {
	return itestkithttpmock.NewRegistry(map[string]itestkit.Handler[*harness]{
		"CreateOrder": createOrderHandler{},
	})
}
