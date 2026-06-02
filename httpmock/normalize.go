package httpmock

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// normalizePlan validates the fixture contract and applies default matching modes.
func normalizePlan(plan Plan) (Plan, error) {
	normalized := Plan{
		Calls:    make([]CallExpectation, len(plan.Calls)),
		Ordering: plan.Ordering,
	}
	if normalized.Ordering == "" {
		normalized.Ordering = OrderingAny
	}
	if normalized.Ordering != OrderingStrict && normalized.Ordering != OrderingAny {
		return Plan{}, fmt.Errorf("field ordering has unsupported value %q; allowed values: strict, any", plan.Ordering)
	}

	for index := range plan.Calls {
		normalizedCall, err := normalizeCall(plan.Calls[index])
		if err != nil {
			return Plan{}, fmt.Errorf("calls[%d]: %w", index, err)
		}
		normalized.Calls[index] = normalizedCall
	}

	return normalized, nil
}

// normalizeCall validates one planned call and applies default matching modes.
func normalizeCall(call CallExpectation) (CallExpectation, error) {
	normalized := call
	normalized.Method = strings.TrimSpace(call.Method)
	normalized.Path = strings.TrimSpace(call.Path)
	if normalized.QueryMode == "" {
		normalized.QueryMode = QueryModeExact
	}
	if normalized.HeadersMode == "" {
		normalized.HeadersMode = HeadersModeExact
	}
	if normalized.CountMode == "" {
		normalized.CountMode = CountModeExact
	}

	if normalized.ExpectedCount < 0 {
		return CallExpectation{}, errors.New("field expected_count must be greater than or equal to 0")
	}
	if normalized.Method == "" {
		return CallExpectation{}, errors.New("field method is required")
	}
	if normalized.Path == "" {
		return CallExpectation{}, errors.New("field path is required")
	}
	if normalized.QueryMode != QueryModeExact && normalized.QueryMode != QueryModeSubset {
		return CallExpectation{}, fmt.Errorf(
			"field query_mode has unsupported value %q; allowed values: exact, subset",
			normalized.QueryMode,
		)
	}
	if normalized.HeadersMode != HeadersModeExact && normalized.HeadersMode != HeadersModeSubset {
		return CallExpectation{}, fmt.Errorf(
			"field headers_mode has unsupported value %q; allowed values: exact, subset",
			normalized.HeadersMode,
		)
	}
	if normalized.CountMode != CountModeExact && normalized.CountMode != CountModeAtLeast {
		return CallExpectation{}, fmt.Errorf(
			"field count_mode has unsupported value %q; allowed values: exact, at_least",
			normalized.CountMode,
		)
	}
	if err := validateRequestBodyFields(normalized); err != nil {
		return CallExpectation{}, err
	}
	if err := validateResponse(normalized.Response); err != nil {
		return CallExpectation{}, fmt.Errorf("field response: %w", err)
	}

	normalized.Query = cloneValues(call.Query)
	normalized.Headers = canonicalizeHeaderValues(call.Headers)
	normalized.Body = cloneRawMessage(call.Body)
	normalized.BodySubset = cloneRawMessage(call.BodySubset)
	normalized.RawBody = cloneStringPointer(call.RawBody)
	normalized.FormBody = cloneValues(call.FormBody)
	normalized.FormBodySubset = cloneValues(call.FormBodySubset)
	normalized.Response = cloneResponse(call.Response)

	return normalized, nil
}

// validateRequestBodyFields checks request body field exclusivity and JSON validity.
func validateRequestBodyFields(call CallExpectation) error {
	setCount := 0
	if hasRawJSON(call.Body) {
		setCount++
		if err := validateStrictJSON(call.Body, "body"); err != nil {
			return err
		}
	}
	if hasRawJSON(call.BodySubset) {
		setCount++
		if err := validateTopLevelJSONObject(call.BodySubset, "body_subset"); err != nil {
			return err
		}
	}
	if call.RawBody != nil {
		setCount++
	}
	if call.FormBody != nil {
		setCount++
	}
	if call.FormBodySubset != nil {
		setCount++
	}
	if setCount > 1 {
		return errors.New("exactly one of fields body, body_subset, raw_body, form_body, and form_body_subset can be set")
	}

	return nil
}

// validateResponse checks response status and response body field exclusivity.
func validateResponse(response Response) error {
	if response.Status < 100 || response.Status > 999 {
		return errors.New("field status must be between 100 and 999")
	}
	if hasRawJSON(response.Body) && response.RawBody != nil {
		return errors.New("fields body and raw_body are mutually exclusive")
	}
	if hasRawJSON(response.Body) {
		if err := validateStrictJSON(response.Body, "body"); err != nil {
			return err
		}
	}

	return nil
}

// validateStrictJSON checks that raw contains one complete JSON value.
func validateStrictJSON(raw json.RawMessage, fieldName string) error {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("field %s: decode json: %w", fieldName, err)
	}
	if err := decoder.Decode(&struct{}{}); err == nil {
		return fmt.Errorf("field %s: decode json: trailing data", fieldName)
	}

	return nil
}

// validateTopLevelJSONObject checks that raw contains one JSON object.
func validateTopLevelJSONObject(raw json.RawMessage, fieldName string) error {
	value, err := decodeStrictJSONAny(raw)
	if err != nil {
		return fmt.Errorf("field %s: %w", fieldName, err)
	}
	if _, ok := value.(map[string]any); !ok {
		return fmt.Errorf("field %s must be a JSON object", fieldName)
	}

	return nil
}

// hasRawJSON reports whether raw contains a non-empty JSON value.
func hasRawJSON(raw json.RawMessage) bool {
	return len(bytes.TrimSpace(raw)) > 0
}

// responseBodyBytes returns the configured response body bytes.
func responseBodyBytes(response Response) []byte {
	if response.RawBody != nil {
		return []byte(*response.RawBody)
	}
	if hasRawJSON(response.Body) {
		return response.Body
	}
	return nil
}

// cloneResponse returns a deep copy of response fixture data.
func cloneResponse(response Response) Response {
	return Response{
		Status:  response.Status,
		Headers: cloneValues(response.Headers),
		Body:    cloneRawMessage(response.Body),
		RawBody: cloneStringPointer(response.RawBody),
	}
}
