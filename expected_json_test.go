package itestkit

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDecodeExpectedJSON_PreservesMarkersAndNumberLexemes verifies that custom handlers can decode matcher templates as JSON-safe values.
func TestDecodeExpectedJSON_PreservesMarkersAndNumberLexemes(t *testing.T) {
	t.Parallel()

	expected, err := DecodeExpectedJSON(json.RawMessage(`{"token":{"$present":true},"price":1.20}`))
	require.NoError(t, err)
	require.Equal(t, map[string]any{
		"token": map[string]any{"$present": true},
		"price": json.Number("1.20"),
	}, expected)
}

// TestMatchExpectedJSON_ExactMode verifies full structure matching with presence markers.
func TestMatchExpectedJSON_ExactMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		expected      any
		actual        any
		errorContains string
	}{
		{
			name:          "present marker accepts any value including null",
			expected:      mustDecodeExpectedJSON(t, `{"id":"1","password":{"$present":true},"expires_at":{"$present":true}}`),
			actual:        mustDecodeExpectedJSON(t, `{"id":"1","password":null,"expires_at":"2026-06-29T12:00:00Z"}`),
			errorContains: "",
		},
		{
			name:          "extra actual field fails",
			expected:      mustDecodeExpectedJSON(t, `{"id":"1"}`),
			actual:        mustDecodeExpectedJSON(t, `{"id":"1","extra":true}`),
			errorContains: "json mismatch",
		},
		{
			name:          "absent marker is forbidden",
			expected:      mustDecodeExpectedJSON(t, `{"deleted_at":{"$absent":true}}`),
			actual:        mustDecodeExpectedJSON(t, `{}`),
			errorContains: "$.deleted_at: absence marker is supported only in partial response mode",
		},
		{
			name:          "presence marker is allowed only as object field value",
			expected:      mustDecodeExpectedJSON(t, `[{"$present":true}]`),
			actual:        mustDecodeExpectedJSON(t, `["any"]`),
			errorContains: "$[0]: presence marker can be used only as an object field value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := MatchExpectedJSON(tt.expected, tt.actual, MatchModeExact)
			if tt.errorContains == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.errorContains)
		})
	}
}

// TestMatchExpectedJSON_PartialMode verifies subset object matching with absence and presence markers.
func TestMatchExpectedJSON_PartialMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		expected      any
		actual        any
		errorContains string
	}{
		{
			name:          "object fields are subset",
			expected:      mustDecodeExpectedJSON(t, `{"profile":{"email":"a@example.test"}}`),
			actual:        mustDecodeExpectedJSON(t, `{"id":"1","profile":{"email":"a@example.test","name":"Ann"}}`),
			errorContains: "",
		},
		{
			name:          "absent marker accepts missing field",
			expected:      mustDecodeExpectedJSON(t, `{"deleted_at":{"$absent":true}}`),
			actual:        mustDecodeExpectedJSON(t, `{"id":"1"}`),
			errorContains: "",
		},
		{
			name:          "absent marker rejects null field",
			expected:      mustDecodeExpectedJSON(t, `{"deleted_at":{"$absent":true}}`),
			actual:        mustDecodeExpectedJSON(t, `{"deleted_at":null}`),
			errorContains: "$.deleted_at: field must be absent",
		},
		{
			name:          "missing regular field reports path",
			expected:      mustDecodeExpectedJSON(t, `{"profile":{"email":"a@example.test"}}`),
			actual:        mustDecodeExpectedJSON(t, `{"profile":{}}`),
			errorContains: "$.profile.email: missing field",
		},
		{
			name:          "arrays require exact length",
			expected:      mustDecodeExpectedJSON(t, `{"items":[{"id":1}]}`),
			actual:        mustDecodeExpectedJSON(t, `{"items":[{"id":1},{"id":2}]}`),
			errorContains: "$.items: array length mismatch",
		},
		{
			name:          "scalars use strict equality",
			expected:      mustDecodeExpectedJSON(t, `{"enabled":true}`),
			actual:        mustDecodeExpectedJSON(t, `{"enabled":"true"}`),
			errorContains: "$.enabled: value mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := MatchExpectedJSON(tt.expected, tt.actual, MatchModePartial)
			if tt.errorContains == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.errorContains)
		})
	}
}

// TestMatchExpectedJSON_RejectsUnsupportedMode verifies that callers cannot silently get exact matching for a misspelled mode.
func TestMatchExpectedJSON_RejectsUnsupportedMode(t *testing.T) {
	t.Parallel()

	err := MatchExpectedJSON(map[string]any{}, map[string]any{}, MatchMode("subset"))
	require.Error(t, err)
	require.Contains(t, err.Error(), `unsupported match mode "subset"`)
}

// mustDecodeExpectedJSON decodes JSON templates for matcher API tests.
func mustDecodeExpectedJSON(t *testing.T, raw string) any {
	t.Helper()

	decoded, err := DecodeExpectedJSON(json.RawMessage(raw))
	require.NoError(t, err)
	return decoded
}
