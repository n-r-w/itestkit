package itestkit

import (
	"encoding/json"
	"errors"
	"fmt"
)

// MatchMode selects how expected JSON is compared with actual JSON-safe data.
type MatchMode string

const (
	// MatchModeExact requires expected and actual JSON structures to match completely.
	MatchModeExact MatchMode = "exact"
	// MatchModePartial requires expected object fields to match as a subset of actual objects.
	MatchModePartial MatchMode = "partial"
)

// DecodeExpectedJSON decodes expected JSON while preserving matcher objects and JSON numbers.
func DecodeExpectedJSON(raw json.RawMessage) (any, error) {
	return decodeRawExpectedResponse(raw)
}

// MatchExpectedJSON compares expected JSON with actual JSON-safe data using marker-aware rules.
func MatchExpectedJSON(expected, actual any, mode MatchMode) error {
	switch mode {
	case MatchModeExact:
		return matchExpectedJSONExact(expected, actual)
	case MatchModePartial:
		return matchExpectedJSONPartial(expected, actual)
	default:
		return fmt.Errorf("unsupported match mode %q", mode)
	}
}

// matchExpectedJSONExact enforces full equality after replacing allowed matchers with actual values.
func matchExpectedJSONExact(expected, actual any) error {
	materializedExpected, materializeErr := materializeExpectedMatchers("$", expected, actual)
	if materializeErr != nil {
		return fmt.Errorf("json comparison failed: %w", materializeErr)
	}

	valuesEqual, valuesDiff, compareErr := compareNormalizedResponses(materializedExpected, actual)
	if compareErr != nil {
		return fmt.Errorf("json comparison failed: %w", compareErr)
	}
	if valuesEqual {
		return nil
	}
	if valuesDiff == "" {
		return errors.New("json mismatch")
	}
	return fmt.Errorf("json mismatch (-want +got):\n%s", valuesDiff)
}

// matchExpectedJSONPartial enforces recursive subset matching for objects and strict matching for arrays and scalars.
func matchExpectedJSONPartial(expected, actual any) error {
	if err := comparePartialResponseValue("$", expected, actual); err != nil {
		return fmt.Errorf("json partial mismatch: %w", err)
	}
	return nil
}
