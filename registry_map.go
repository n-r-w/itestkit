package itestkit

import (
	"fmt"
	"maps"
	"sort"
)

// MapRegistry stores step handlers and implements HandlerRegistry.
type MapRegistry[C any] struct {
	handlers map[string]Handler[C]
}

// compileTimeMapRegistryCheck verifies the HandlerRegistry implementation.
var _ HandlerRegistry[any] = (*MapRegistry[any])(nil)

// NewMapRegistry creates a map-based handler registry with protective copying.
func NewMapRegistry[C any](handlers map[string]Handler[C]) MapRegistry[C] {
	copiedHandlers := make(map[string]Handler[C], len(handlers))
	maps.Copy(copiedHandlers, handlers)

	return MapRegistry[C]{
		handlers: copiedHandlers,
	}
}

// Resolve returns a handler by name.
func (registry MapRegistry[C]) Resolve(name string) (Handler[C], error) {
	handler, exists := registry.handlers[name]
	if !exists {
		return nil, fmt.Errorf("unsupported handler %q", name)
	}

	return handler, nil
}

// Supported returns a sorted list of supported handlers.
func (registry MapRegistry[C]) Supported() []string {
	names := make([]string, 0, len(registry.handlers))
	for name := range registry.handlers {
		names = append(names, name)
	}
	sort.Strings(names)

	return names
}
