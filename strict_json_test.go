package itestkit

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

type strictJSONPayload struct {
	Name string `json:"name"`
}

// TestDecodeStrictJSON_Success tests the underlying successful strict decode.
func TestDecodeStrictJSON_Success(t *testing.T) {
	t.Parallel()

	payload := strictJSONPayload{Name: ""}
	err := DecodeStrictJSON(json.RawMessage(`{"name":"ok"}`), &payload)
	require.NoError(t, err)
	require.Equal(t, strictJSONPayload{Name: "ok"}, payload)
}

// TestDecodeStrictJSON_RejectsUnknownFields checks for rejection on unnecessary fields.
func TestDecodeStrictJSON_RejectsUnknownFields(t *testing.T) {
	t.Parallel()

	payload := strictJSONPayload{Name: ""}
	err := DecodeStrictJSON(json.RawMessage(`{"name":"ok","extra":1}`), &payload)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown field")
}

// TestDecodeStrictJSON_RejectsTrailingData checks for a rejection on trailing JSON.
func TestDecodeStrictJSON_RejectsTrailingData(t *testing.T) {
	t.Parallel()

	payload := strictJSONPayload{Name: ""}
	err := DecodeStrictJSON(json.RawMessage(`{"name":"ok"} {}`), &payload)
	require.Error(t, err)
	require.Contains(t, err.Error(), "trailing data")
}
