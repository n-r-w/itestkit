package httpmock

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"sort"

	"github.com/n-r-w/itestkit/internal/jsonsubset"
)

// evaluate checks observed requests against the planned call set.
func evaluate(plan Plan, observed []ObservedRequest, final bool) (CheckResult, error) {
	if plan.Ordering == OrderingStrict {
		return evaluateStrict(plan, observed, final)
	}
	return evaluateAny(plan, observed, final)
}

// evaluateStrict checks observed requests against the expanded plan order.
func evaluateStrict(plan Plan, observed []ObservedRequest, final bool) (CheckResult, error) {
	expected := expandCalls(plan.Calls)
	limit := len(observed)
	if len(expected) < limit {
		limit = len(expected)
	}

	result := CheckResult{
		MatchedCount:       0,
		ObservedRequests:   cloneObservedRequests(observed),
		LastMismatchReason: "",
	}
	for index := range limit {
		matched, reason := matchesCall(expected[index], observed[index])
		if !matched {
			result.LastMismatchReason = reason
			return result, fmt.Errorf("strict ordering mismatch at request %d: %s", index, reason)
		}
		result.MatchedCount++
	}

	if len(observed) > len(expected) {
		var err error
		result, err = evaluateStrictExtraObserved(result, expected, observed, final)
		if err != nil {
			return result, err
		}
	}

	if !final && result.MatchedCount < len(expected) {
		return result, fmt.Errorf("expected matched_count=%d, got %d", len(expected), result.MatchedCount)
	}
	if final && len(observed) < len(expected) {
		return result, fmt.Errorf("expected observed request count=%d, got %d", len(expected), len(observed))
	}

	return result, nil
}

// evaluateStrictExtraObserved checks requests that exceed the expanded strict plan.
func evaluateStrictExtraObserved(
	result CheckResult,
	expected []CallExpectation,
	observed []ObservedRequest,
	final bool,
) (CheckResult, error) {
	lastExpected := expected[len(expected)-1]
	if lastExpected.CountMode != CountModeAtLeast && final {
		return result, fmt.Errorf("expected observed request count=%d, got %d", len(expected), len(observed))
	}
	if lastExpected.CountMode != CountModeAtLeast {
		return result, nil
	}

	for index := len(expected); index < len(observed); index++ {
		matched, reason := matchesCall(lastExpected, observed[index])
		if !matched {
			result.LastMismatchReason = reason
			return result, fmt.Errorf("strict ordering mismatch at request %d: %s", index, reason)
		}
		result.MatchedCount++
	}
	return result, nil
}

// evaluateAny checks observed requests without requiring order.
func evaluateAny(plan Plan, observed []ObservedRequest, final bool) (CheckResult, error) {
	used := make([]bool, len(observed))
	result := CheckResult{
		MatchedCount:       0,
		ObservedRequests:   cloneObservedRequests(observed),
		LastMismatchReason: "",
	}

	for callIndex := range plan.Calls {
		call := plan.Calls[callIndex]
		if call.ExpectedCount == 0 {
			continue
		}

		matchedForCall := 0
		for index, request := range observed {
			if used[index] {
				continue
			}
			matched, reason := matchesCall(call, request)
			if !matched {
				result.LastMismatchReason = reason
				continue
			}
			used[index] = true
			matchedForCall++
			result.MatchedCount++
			if call.CountMode == CountModeExact && matchedForCall == call.ExpectedCount {
				break
			}
		}
		if matchedForCall < call.ExpectedCount {
			message := fmt.Sprintf("expected matched_count=%d, got %d", expectedTotal(plan.Calls), result.MatchedCount)
			if result.LastMismatchReason != "" {
				message += ": " + result.LastMismatchReason
			}
			return result, errors.New(message)
		}
	}

	if final {
		for index, isUsed := range used {
			if !isUsed {
				result.LastMismatchReason = describeObservedRequest(observed[index])
				return result, fmt.Errorf("unexpected request: %s", result.LastMismatchReason)
			}
		}
	}

	return result, nil
}

// responseForRequest returns the planned response for the current observed request.
func responseForRequest(plan Plan, observed []ObservedRequest, current ObservedRequest) (Response, bool) {
	if plan.Ordering == OrderingStrict {
		return strictResponseForRequest(plan, observed)
	}
	return anyResponseForRequest(plan, observed, current)
}

// strictResponseForRequest checks the current request against its strict-order slot.
func strictResponseForRequest(plan Plan, observed []ObservedRequest) (Response, bool) {
	expected := expandCalls(plan.Calls)
	index := len(observed) - 1
	if index < 0 {
		return Response{}, false
	}
	if index >= len(expected) {
		lastExpected := expected[len(expected)-1]
		if lastExpected.CountMode != CountModeAtLeast {
			return Response{}, false
		}
		matched, _ := matchesCall(lastExpected, observed[index])
		if !matched {
			return Response{}, false
		}
		return lastExpected.Response, true
	}
	matched, _ := matchesCall(expected[index], observed[index])
	if !matched {
		return Response{}, false
	}
	return expected[index].Response, true
}

// anyResponseForRequest returns a response for any matching expectation with remaining expected count.
func anyResponseForRequest(plan Plan, observed []ObservedRequest, current ObservedRequest) (Response, bool) {
	for callIndex := range plan.Calls {
		call := plan.Calls[callIndex]
		matched, _ := matchesCall(call, current)
		if !matched {
			continue
		}
		seen := 0
		for _, request := range observed {
			matchedSeen, _ := matchesCall(call, request)
			if matchedSeen {
				seen++
			}
		}
		if call.CountMode == CountModeAtLeast || seen <= call.ExpectedCount {
			return call.Response, true
		}
	}

	return Response{}, false
}

// matchesCall checks whether the observed request satisfies one planned call.
func matchesCall(call CallExpectation, request ObservedRequest) (matched bool, mismatchReason string) {
	if request.Method != call.Method {
		return false, fmt.Sprintf("method mismatch: expected %s, got %s", call.Method, request.Method)
	}
	if request.Path != call.Path {
		return false, fmt.Sprintf("path mismatch: expected %s, got %s", call.Path, request.Path)
	}
	if call.Query != nil {
		queryMatched, reason := matchValues("query", string(call.QueryMode), request.Query, call.Query)
		if !queryMatched {
			return false, reason
		}
	}
	headersMatched, reason := matchHeaders(call, request)
	if !headersMatched {
		return false, reason
	}
	bodyMatched, reason := matchBody(call, request)
	if !bodyMatched {
		return false, reason
	}

	return true, ""
}

// matchHeaders checks value-based and presence-only header expectations against one observed request.
func matchHeaders(call CallExpectation, request ObservedRequest) (matched bool, mismatchReason string) {
	if call.Headers == nil && len(call.HeadersPresent) == 0 {
		return true, ""
	}
	actualHeaders := canonicalizeHeaderValues(request.Headers)
	if call.Headers != nil {
		headersMatched, reason := matchValues("headers", string(call.HeadersMode), actualHeaders, call.Headers)
		if !headersMatched {
			return false, reason
		}
	}
	return matchHeaderPresence(actualHeaders, call.HeadersPresent)
}

// matchHeaderPresence checks only that required headers exist, without comparing their values.
func matchHeaderPresence(actual map[string][]string, expected []string) (matched bool, mismatchReason string) {
	for _, headerName := range expected {
		if _, exists := actual[headerName]; !exists {
			return false, fmt.Sprintf("headers_present mismatch: header %q is missing", headerName)
		}
	}
	return true, ""
}

// matchValues checks exact or subset equality for repeated string values.
func matchValues(
	field string,
	mode string,
	actual map[string][]string,
	expected map[string][]string,
) (matched bool, mismatchReason string) {
	if mode == "exact" && len(actual) != len(expected) {
		return false, fmt.Sprintf("%s mismatch: expected %d keys, got %d", field, len(expected), len(actual))
	}
	for key, expectedValues := range expected {
		actualValues, exists := actual[key]
		if !exists {
			return false, fmt.Sprintf("%s mismatch: key %q is missing", field, key)
		}
		if !sameStringMultiset(actualValues, expectedValues) {
			return false, fmt.Sprintf("%s mismatch: key %q values differ", field, key)
		}
	}

	return true, ""
}

// matchBody checks request body fields when the plan defines one.
func matchBody(call CallExpectation, request ObservedRequest) (matched bool, mismatchReason string) {
	switch {
	case hasRawJSON(call.Body):
		return matchExactJSONBody(call.Body, request.Body)
	case hasRawJSON(call.BodySubset):
		return matchSubsetJSONBody(call.BodySubset, request.Body)
	case call.RawBody != nil:
		if request.Body != *call.RawBody {
			return false, fmt.Sprintf("raw_body mismatch: expected %q, got %q", *call.RawBody, request.Body)
		}
	case call.FormBody != nil:
		return matchFormBody("form_body", QueryModeExact, call.FormBody, request.Body)
	case call.FormBodySubset != nil:
		return matchFormBody("form_body_subset", QueryModeSubset, call.FormBodySubset, request.Body)
	}

	return true, ""
}

// matchFormBody checks application/x-www-form-urlencoded bodies without depending on parameter order.
func matchFormBody(
	field string,
	mode QueryMode,
	expected map[string][]string,
	actualRaw string,
) (matched bool, mismatchReason string) {
	actual, err := url.ParseQuery(actualRaw)
	if err != nil {
		return false, fmt.Sprintf("%s mismatch: parse actual form body: %v", field, err)
	}
	return matchValues(field, string(mode), actual, expected)
}

// matchExactJSONBody checks strict decoded JSON equality.
func matchExactJSONBody(expected json.RawMessage, actualRaw string) (matched bool, mismatchReason string) {
	actual, err := decodeStrictJSONAny([]byte(actualRaw))
	if err != nil {
		return false, fmt.Sprintf("body mismatch: decode actual json: %v", err)
	}
	expectedValue, err := decodeStrictJSONAny(expected)
	if err != nil {
		return false, fmt.Sprintf("body mismatch: decode expected json: %v", err)
	}
	if reflect.DeepEqual(actual, expectedValue) {
		return true, ""
	}
	return false, "body mismatch: json values differ"
}

// matchSubsetJSONBody checks strict decoded JSON subset matching.
func matchSubsetJSONBody(expected json.RawMessage, actualRaw string) (matched bool, mismatchReason string) {
	actual, err := decodeStrictJSONAny([]byte(actualRaw))
	if err != nil {
		return false, fmt.Sprintf("body_subset mismatch: decode actual json: %v", err)
	}
	expectedValue, err := decodeStrictJSONAny(expected)
	if err != nil {
		return false, fmt.Sprintf("body_subset mismatch: decode expected json: %v", err)
	}
	mismatchPath, mismatchReason, matched := jsonsubset.Match(actual, expectedValue, "$")
	if !matched {
		return false, fmt.Sprintf("body_subset mismatch at %s: %s", mismatchPath, mismatchReason)
	}

	return true, ""
}

// decodeStrictJSONAny decodes one JSON value and preserves JSON numbers.
func decodeStrictJSONAny(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("decode json: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err == nil {
		return nil, errors.New("decode json: trailing data")
	}

	return value, nil
}

// expandCalls expands expected_count into an ordered call sequence.
func expandCalls(calls []CallExpectation) []CallExpectation {
	expanded := make([]CallExpectation, 0, expectedTotal(calls))
	for callIndex := range calls {
		call := calls[callIndex]
		for range call.ExpectedCount {
			expanded = append(expanded, call)
		}
	}
	return expanded
}

// expectedTotal returns the total expected request count.
func expectedTotal(calls []CallExpectation) int {
	total := 0
	for callIndex := range calls {
		total += calls[callIndex].ExpectedCount
	}
	return total
}

// sameStringMultiset checks equality while ignoring value order.
func sameStringMultiset(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	actualCopy := append([]string(nil), actual...)
	expectedCopy := append([]string(nil), expected...)
	sort.Strings(actualCopy)
	sort.Strings(expectedCopy)
	return reflect.DeepEqual(actualCopy, expectedCopy)
}

// describeObservedRequest returns a compact request description for mismatch messages.
func describeObservedRequest(request ObservedRequest) string {
	return fmt.Sprintf("%s %s", request.Method, request.Path)
}
