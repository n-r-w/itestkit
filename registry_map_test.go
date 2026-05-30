package itestkit

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestNewMapRegistry_Resolve checks the successful resolution of the handler by name.
func TestNewMapRegistry_Resolve(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)

	expectedHandler := NewMockHandler[string](ctrl)
	registry := NewMapRegistry(map[string]Handler[string]{
		"A": expectedHandler,
	})

	resolvedHandler, err := registry.Resolve("A")
	require.NoError(t, err)
	require.Same(t, expectedHandler, resolvedHandler)
}

// TestNewMapRegistry_ResolveUnknown tests for an error on an unknown handler name.
func TestNewMapRegistry_ResolveUnknown(t *testing.T) {
	t.Parallel()

	registry := NewMapRegistry[string](nil)

	resolvedHandler, err := registry.Resolve("unknown")
	require.Nil(t, resolvedHandler)
	require.EqualError(t, err, `unsupported handler "unknown"`)
}

// TestNewMapRegistry_SupportedSorted tests the sorting of the list of supported names.
func TestNewMapRegistry_SupportedSorted(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)

	registry := NewMapRegistry(map[string]Handler[string]{
		"B": NewMockHandler[string](ctrl),
		"A": NewMockHandler[string](ctrl),
		"C": NewMockHandler[string](ctrl),
	})

	require.Equal(t, []string{"A", "B", "C"}, registry.Supported())
}

// TestNewMapRegistry_DefensiveCopy checks the registry's isolation from external mutations of the source map.
func TestNewMapRegistry_DefensiveCopy(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)

	originalHandler := NewMockHandler[string](ctrl)
	replacementHandler := NewMockHandler[string](ctrl)
	sourceHandlers := map[string]Handler[string]{
		"A": originalHandler,
	}

	registry := NewMapRegistry(sourceHandlers)
	sourceHandlers["A"] = replacementHandler

	resolvedHandler, err := registry.Resolve("A")
	require.NoError(t, err)
	require.Same(t, originalHandler, resolvedHandler)
}
