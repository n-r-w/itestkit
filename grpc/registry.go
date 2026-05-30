package grpc

import (
	"fmt"
	"maps"
	"sort"

	"github.com/n-r-w/itestkit"
	"google.golang.org/grpc"
)

// Registry stores a set of gRPC specs and implements itestkit.HandlerRegistry.
type Registry[C any] struct {
	specs map[string]HandlerSpec[C]
}

// compileTimeRegistryCheck verifies the implementation of itestkit.HandlerRegistry.
var _ itestkit.HandlerRegistry[any] = (*Registry[any])(nil)

// NewRegistry creates a registry handler spec for gRPC integration.
func NewRegistry[C any](specs map[string]HandlerSpec[C]) Registry[C] {
	copiedSpecs := make(map[string]HandlerSpec[C], len(specs))
	maps.Copy(copiedSpecs, specs)

	return Registry[C]{
		specs: copiedSpecs,
	}
}

// Resolve returns a handler by name.
func (reg Registry[C]) Resolve(name string) (itestkit.Handler[C], error) {
	spec, exists := reg.specs[name]
	if !exists {
		return nil, fmt.Errorf("unsupported handler %q", name)
	}
	if spec.NewRequest == nil {
		return nil, fmt.Errorf("handler %q has nil NewRequest", name)
	}
	if spec.NewResponse == nil {
		return nil, fmt.Errorf("handler %q has nil NewResponse", name)
	}
	if spec.Invoke == nil {
		return nil, fmt.Errorf("handler %q has nil Invoke", name)
	}
	return handlerAdapter[C]{spec: spec}, nil
}

// Supported returns a sorted list of supported handlers.
func (reg Registry[C]) Supported() []string {
	names := make([]string, 0, len(reg.specs))
	for name := range reg.specs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ServiceMethodNames returns a sorted list of service methods.
func ServiceMethodNames(desc *grpc.ServiceDesc) []string {
	if desc == nil {
		return nil
	}

	methods := make([]string, 0, len(desc.Methods))
	for _, method := range desc.Methods {
		methods = append(methods, method.MethodName)
	}
	sort.Strings(methods)
	return methods
}
