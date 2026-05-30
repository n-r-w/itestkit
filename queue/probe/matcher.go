package probe

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"reflect"
	"strings"

	"github.com/google/go-cmp/cmp"
)

const (
	pendingZeroCountAwaitReason = "await retry window is still open"
)

// NormalizeExpectation validates the contract and applies default values.
func NormalizeExpectation(expectation OutboundExpectation) (OutboundExpectation, error) {
	normalized := cloneExpectation(expectation)
	if err := validateExpectationTopic(normalized.Topic); err != nil {
		return OutboundExpectation{}, err
	}
	if err := validateExpectationCount(normalized.ExpectedCount); err != nil {
		return OutboundExpectation{}, err
	}
	if err := applyExpectationDefaults(&normalized); err != nil {
		return OutboundExpectation{}, err
	}
	if err := validateExpectationPayloads(normalized); err != nil {
		return OutboundExpectation{}, err
	}

	return normalized, nil
}

// cloneExpectation copies the expectation and performs minimal normalization without validation.
func cloneExpectation(expectation OutboundExpectation) OutboundExpectation {
	return OutboundExpectation{
		Topic:         strings.TrimSpace(expectation.Topic),
		Key:           cloneOptionalString(expectation.Key),
		Headers:       cloneHeaders(expectation.Headers),
		HeadersMode:   expectation.HeadersMode,
		ExpectedCount: expectation.ExpectedCount,
		Payload:       cloneRawMessage(expectation.Payload),
		PayloadSubset: cloneRawMessage(expectation.PayloadSubset),
		Ordering:      expectation.Ordering,
	}
}

// validateExpectationTopic checks whether topic is required after trim transformation.
func validateExpectationTopic(topic string) error {
	if topic == "" {
		return errors.New("field topic is required")
	}

	return nil
}

// validateExpectationCount disallows negative expected_count.
func validateExpectationCount(expectedCount int) error {
	if expectedCount < 0 {
		return errors.New("field expected_count must be >= 0")
	}

	return nil
}

// applyExpectationDefaults sets default values ​​and validates enum fields.
func applyExpectationDefaults(expectation *OutboundExpectation) error {
	if expectation.HeadersMode == "" {
		expectation.HeadersMode = HeadersModeExact
	}
	if expectation.HeadersMode != HeadersModeExact && expectation.HeadersMode != HeadersModeSubset {
		return fmt.Errorf(
			"field headers_mode has unsupported value %q; allowed values: exact, subset",
			expectation.HeadersMode,
		)
	}

	if expectation.Ordering == "" {
		expectation.Ordering = OrderingAny
	}
	if expectation.Ordering != OrderingStrict && expectation.Ordering != OrderingAny {
		return fmt.Errorf(
			"field ordering has unsupported value %q; allowed values: strict, any",
			expectation.Ordering,
		)
	}

	return nil
}

// validateExpectationPayloads checks the contract between expected_count and payload fields.
func validateExpectationPayloads(expectation OutboundExpectation) error {
	hasPayload := hasRawJSON(expectation.Payload)
	hasPayloadSubset := hasRawJSON(expectation.PayloadSubset)
	if expectation.ExpectedCount == 0 {
		if hasPayload || hasPayloadSubset {
			return errors.New("fields payload and payload_subset are forbidden when expected_count=0")
		}

		return nil
	}

	if hasPayload == hasPayloadSubset {
		return errors.New("exactly one of fields payload and payload_subset must be set when expected_count>0")
	}
	if err := validateExpectationPayload(expectation.Payload, "payload"); err != nil {
		return err
	}
	if err := validateExpectationPayloadSubset(expectation.PayloadSubset); err != nil {
		return err
	}

	return nil
}

// validateExpectationPayload checks the strict JSON payload for accurate comparison.
func validateExpectationPayload(payload json.RawMessage, fieldName string) error {
	if !hasRawJSON(payload) {
		return nil
	}

	if _, err := decodeStrictJSONAny(payload); err != nil {
		return fmt.Errorf("field %s: %w", fieldName, err)
	}

	return nil
}

// validateExpectationPayloadSubset checks that the subset payload is a JSON object.
func validateExpectationPayloadSubset(payloadSubset json.RawMessage) error {
	if !hasRawJSON(payloadSubset) {
		return nil
	}

	subsetRaw, err := decodeStrictJSONAny(payloadSubset)
	if err != nil {
		return fmt.Errorf("field payload_subset: %w", err)
	}
	if _, ok := subsetRaw.(map[string]any); !ok {
		return errors.New("field payload_subset must be a JSON object")
	}

	return nil
}

// EvaluateAwait checks the intermediate state of the await step.
func EvaluateAwait(expectation OutboundExpectation, observed []Message, windowExpired bool) (CheckResult, error) {
	result, err := evaluate(expectation, observed)
	if err != nil {
		return result, err
	}

	if expectation.ExpectedCount > 0 {
		if result.MatchedCount >= expectation.ExpectedCount {
			return result, nil
		}

		reason := fmt.Sprintf(
			"await condition is not met: matched_count=%d, expected_count=%d",
			result.MatchedCount,
			expectation.ExpectedCount,
		)
		if result.LastMismatchReason != "" {
			reason = fmt.Sprintf("%s; last mismatch: %s", reason, result.LastMismatchReason)
		}
		result.LastMismatchReason = reason
		return result, errors.New(result.LastMismatchReason)
	}

	if result.MatchedCount > 0 {
		result.LastMismatchReason = fmt.Sprintf(
			"await condition failed for expected_count=0: first matching message offset=%d",
			result.ObservedMessages[0].Offset,
		)
		return result, errors.New(result.LastMismatchReason)
	}

	if !windowExpired {
		result.LastMismatchReason = pendingZeroCountAwaitReason
		return result, errors.New(result.LastMismatchReason)
	}

	return result, nil
}

// EvaluateVerify checks the final state of the verify step.
func EvaluateVerify(expectation OutboundExpectation, observed []Message) (CheckResult, error) {
	result, err := evaluate(expectation, observed)
	if err != nil {
		return result, err
	}

	if expectation.ExpectedCount == 0 {
		if result.MatchedCount == 0 {
			return result, nil
		}

		result.LastMismatchReason = fmt.Sprintf(
			"verify expected zero messages, got %d matches",
			result.MatchedCount,
		)
		return result, errors.New(result.LastMismatchReason)
	}

	if expectation.Ordering == OrderingStrict {
		strictErr := evaluateStrictOrdering(expectation, result.ObservedMessages)
		if strictErr != nil {
			result.LastMismatchReason = strictErr.Error()
			return result, strictErr
		}
		return result, nil
	}

	if result.MatchedCount != expectation.ExpectedCount {
		reason := fmt.Sprintf(
			"verify expected matched_count=%d, got %d",
			expectation.ExpectedCount,
			result.MatchedCount,
		)
		if result.LastMismatchReason != "" {
			reason = fmt.Sprintf("%s; last mismatch: %s", reason, result.LastMismatchReason)
		}
		result.LastMismatchReason = reason
		return result, errors.New(result.LastMismatchReason)
	}

	return result, nil
}

// evaluate filters messages by topic and counts matches by contract.
func evaluate(expectation OutboundExpectation, observed []Message) (CheckResult, error) {
	normalized, err := NormalizeExpectation(expectation)
	if err != nil {
		return CheckResult{}, err
	}

	topicMessages := filterMessagesByTopic(observed, normalized.Topic)
	matchedCount := 0
	lastMismatchReason := ""

	for _, message := range topicMessages {
		matched, mismatchReason, matchErr := matchesExpectation(message, normalized)
		if matchErr != nil {
			return CheckResult{}, matchErr
		}
		if matched {
			matchedCount++
			continue
		}
		if mismatchReason != "" {
			lastMismatchReason = mismatchReason
		}
	}

	return CheckResult{
		MatchedCount:       matchedCount,
		ObservedMessages:   cloneMessages(topicMessages),
		LastMismatchReason: lastMismatchReason,
	}, nil
}

// evaluateStrictOrdering checks the strict order and the absence of unnecessary messages.
func evaluateStrictOrdering(expectation OutboundExpectation, observedByTopic []Message) error {
	if len(observedByTopic) < expectation.ExpectedCount {
		return fmt.Errorf(
			"verify strict ordering expected at least %d messages, got %d",
			expectation.ExpectedCount,
			len(observedByTopic),
		)
	}

	for index := range expectation.ExpectedCount {
		matched, mismatchReason, err := matchesExpectation(observedByTopic[index], expectation)
		if err != nil {
			return err
		}
		if matched {
			continue
		}
		if mismatchReason == "" {
			mismatchReason = "message does not satisfy expectation"
		}
		return fmt.Errorf("verify strict ordering mismatch at index=%d: %s", index, mismatchReason)
	}

	if len(observedByTopic) > expectation.ExpectedCount {
		return fmt.Errorf(
			"verify strict ordering expected exactly %d messages, got %d",
			expectation.ExpectedCount,
			len(observedByTopic),
		)
	}

	return nil
}

// matchesExpectation performs a complete comparison of a single message against the expectations contract.
func matchesExpectation(
	message Message,
	expectation OutboundExpectation,
) (matched bool, reason string, err error) {
	if expectation.Key != nil {
		if message.Key == nil {
			return false, "key mismatch: message key is nil", nil
		}
		if *message.Key != *expectation.Key {
			return false, fmt.Sprintf("key mismatch: expected %q, got %q", *expectation.Key, *message.Key), nil
		}
	}

	if len(expectation.Headers) > 0 {
		headersMatched, headersReason := matchesHeaders(message.Headers, expectation.Headers, expectation.HeadersMode)
		if !headersMatched {
			return false, headersReason, nil
		}
	}

	if hasRawJSON(expectation.Payload) {
		payloadMatched, payloadReason, payloadErr := matchesPayloadExact(message.Payload, expectation.Payload)
		if payloadErr != nil {
			return false, "", payloadErr
		}
		if !payloadMatched {
			return false, payloadReason, nil
		}
	}

	if hasRawJSON(expectation.PayloadSubset) {
		payloadMatched, payloadReason, payloadErr := matchesPayloadSubset(message.Payload, expectation.PayloadSubset)
		if payloadErr != nil {
			return false, "", payloadErr
		}
		if !payloadMatched {
			return false, payloadReason, nil
		}
	}

	return true, "", nil
}

// matchesHeaders checks headers in exact/subset mode.
func matchesHeaders(
	actual, expected map[string]string,
	mode HeadersMode,
) (matched bool, mismatchReason string) {
	if actual == nil {
		actual = map[string]string{}
	}

	if mode == HeadersModeExact {
		if len(actual) != len(expected) {
			return false, fmt.Sprintf("headers mismatch: expected %d headers, got %d", len(expected), len(actual))
		}
	}

	for key, expectedValue := range expected {
		actualValue, exists := actual[key]
		if !exists {
			return false, fmt.Sprintf("headers mismatch: missing key %q", key)
		}
		if actualValue != expectedValue {
			return false, fmt.Sprintf("headers mismatch for key %q: expected %q, got %q", key, expectedValue, actualValue)
		}
	}

	return true, ""
}

// matchesPayloadExact compares payload by exact JSON equality.
func matchesPayloadExact(actualRaw, expectedRaw json.RawMessage) (matched bool, mismatchReason string, err error) {
	actual, err := decodeStrictJSONAny(actualRaw)
	if err != nil {
		return false, "", fmt.Errorf("decode actual payload: %w", err)
	}
	expected, err := decodeStrictJSONAny(expectedRaw)
	if err != nil {
		return false, "", fmt.Errorf("decode expected payload: %w", err)
	}

	if reflect.DeepEqual(actual, expected) {
		return true, "", nil
	}

	diff := cmp.Diff(expected, actual)
	if diff == "" {
		return false, "payload mismatch", nil
	}

	return false, "payload mismatch (-want +got):\n" + diff, nil
}

// matchesPayloadSubset checks that payload contains the expected subset of JSON fields.
func matchesPayloadSubset(actualRaw, subsetRaw json.RawMessage) (matched bool, mismatchReason string, err error) {
	actual, err := decodeStrictJSONAny(actualRaw)
	if err != nil {
		return false, "", fmt.Errorf("decode actual payload: %w", err)
	}
	actualMap, ok := actual.(map[string]any)
	if !ok {
		return false, "payload subset mismatch: actual payload is not object", nil
	}

	subset, err := decodeStrictJSONAny(subsetRaw)
	if err != nil {
		return false, "", fmt.Errorf("decode payload subset: %w", err)
	}
	subsetMap, ok := subset.(map[string]any)
	if !ok {
		return false, "", errors.New("payload subset is not a JSON object")
	}

	mismatchPath, mismatchReason, matched := jsonSubsetMatch(actualMap, subsetMap, "payload")
	if matched {
		return true, "", nil
	}

	return false, fmt.Sprintf("payload subset mismatch at %s: %s", mismatchPath, mismatchReason), nil
}

// jsonSubsetMatch recursively compares the expected subset with the actual value.
func jsonSubsetMatch(actual, expected any, path string) (mismatchPath, mismatchReason string, matched bool) {
	switch expectedTyped := expected.(type) {
	case map[string]any:
		actualMap, ok := actual.(map[string]any)
		if !ok {
			return path, "actual value is not object", false
		}

		for key, expectedValue := range expectedTyped {
			actualValue, exists := actualMap[key]
			if !exists {
				return path + "." + key, "key is missing", false
			}

			nestedPath, nestedReason, nestedMatched := jsonSubsetMatch(actualValue, expectedValue, path+"."+key)
			if !nestedMatched {
				return nestedPath, nestedReason, false
			}
		}

		return "", "", true
	case []any:
		actualSlice, ok := actual.([]any)
		if !ok {
			return path, "actual value is not array", false
		}
		if len(actualSlice) < len(expectedTyped) {
			reason := fmt.Sprintf(
				"actual array length %d is less than expected %d",
				len(actualSlice),
				len(expectedTyped),
			)
			return path, reason, false
		}

		for index, expectedValue := range expectedTyped {
			nestedPath, nestedReason, nestedMatched := jsonSubsetMatch(
				actualSlice[index],
				expectedValue,
				fmt.Sprintf("%s[%d]", path, index),
			)
			if !nestedMatched {
				return nestedPath, nestedReason, false
			}
		}

		return "", "", true
	default:
		if reflect.DeepEqual(actual, expected) {
			return "", "", true
		}
		return path, fmt.Sprintf("expected %v, got %v", expected, actual), false
	}
}

// decodeStrictJSONAny decodes JSON to any and rejects trailing data.
func decodeStrictJSONAny(raw json.RawMessage) (any, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, errors.New("json payload is empty")
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode json: %w", err)
	}

	var trailing any
	err := decoder.Decode(&trailing)
	if err == nil {
		return nil, errors.New("decode json: trailing data")
	}
	if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("decode json: trailing data: %w", err)
	}

	return decoded, nil
}

// filterMessagesByTopic leaves only messages from the expected topic.
func filterMessagesByTopic(observed []Message, topic string) []Message {
	filtered := make([]Message, 0, len(observed))
	for _, message := range observed {
		if message.Topic == topic {
			filtered = append(filtered, message)
		}
	}

	return filtered
}

// hasRawJSON determines whether raw JSON contains a non-empty value.
func hasRawJSON(raw json.RawMessage) bool {
	return len(bytes.TrimSpace(raw)) > 0
}

// cloneHeaders copies the map of headers.
func cloneHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}

	cloned := make(map[string]string, len(headers))
	maps.Copy(cloned, headers)
	return cloned
}

// cloneRawMessage copies the JSON payload.
func cloneRawMessage(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return bytes.Clone(raw)
}

// cloneOptionalString copies an optional string pointer.
func cloneOptionalString(value *string) *string {
	if value == nil {
		return nil
	}

	cloned := *value
	return &cloned
}

// cloneMessages copies the list of observed messages.
func cloneMessages(messages []Message) []Message {
	if len(messages) == 0 {
		return nil
	}

	cloned := make([]Message, 0, len(messages))
	for _, message := range messages {
		cloned = append(cloned, Message{
			Topic:   message.Topic,
			Key:     cloneOptionalString(message.Key),
			Headers: cloneHeaders(message.Headers),
			Payload: cloneRawMessage(message.Payload),
			Offset:  message.Offset,
		})
	}

	return cloned
}
