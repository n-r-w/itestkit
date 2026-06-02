package jsonsubset

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMatch_ObjectSubsetAllowsExtraActualFields checks recursive object subset matching.
func TestMatch_ObjectSubsetAllowsExtraActualFields(t *testing.T) {
	t.Parallel()

	actual := map[string]any{
		"meta": map[string]any{
			"source": "api",
			"trace":  "trace-1",
		},
	}
	expected := map[string]any{
		"meta": map[string]any{
			"source": "api",
		},
	}

	mismatchPath, mismatchReason, matched := Match(actual, expected, "$.")

	assert.True(t, matched)
	assert.Empty(t, mismatchPath)
	assert.Empty(t, mismatchReason)
}

// TestMatch_ArraySubsetUsesPositionalPrefix checks array prefix matching.
func TestMatch_ArraySubsetUsesPositionalPrefix(t *testing.T) {
	t.Parallel()

	actual := []any{"first", "second"}
	expected := []any{"first"}

	mismatchPath, mismatchReason, matched := Match(actual, expected, "$")

	assert.True(t, matched)
	assert.Empty(t, mismatchPath)
	assert.Empty(t, mismatchReason)
}

// TestMatch_ReportsMissingObjectKey checks missing-key diagnostics.
func TestMatch_ReportsMissingObjectKey(t *testing.T) {
	t.Parallel()

	mismatchPath, mismatchReason, matched := Match(map[string]any{}, map[string]any{"id": "1"}, "$")

	assert.False(t, matched)
	assert.Equal(t, "$.id", mismatchPath)
	assert.Equal(t, "key is missing", mismatchReason)
}
