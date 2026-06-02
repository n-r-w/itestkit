// Package jsonsubset provides structural JSON subset matching for internal helpers.
package jsonsubset

import (
	"fmt"
	"reflect"
)

// Match checks whether expected is a structural subset of actual.
func Match(actual, expected any, path string) (mismatchPath, mismatchReason string, matched bool) {
	switch expectedTyped := expected.(type) {
	case map[string]any:
		return matchObject(actual, expectedTyped, path)
	case []any:
		return matchArray(actual, expectedTyped, path)
	default:
		return matchScalar(actual, expected, path)
	}
}

// matchObject requires all expected object fields to exist and match recursively.
func matchObject(actual any, expected map[string]any, path string) (mismatchPath, mismatchReason string, matched bool) {
	actualMap, ok := actual.(map[string]any)
	if !ok {
		return path, "actual value is not object", false
	}

	for key, expectedValue := range expected {
		actualValue, exists := actualMap[key]
		if !exists {
			return path + "." + key, "key is missing", false
		}

		nestedPath, nestedReason, nestedMatched := Match(actualValue, expectedValue, path+"."+key)
		if !nestedMatched {
			return nestedPath, nestedReason, false
		}
	}

	return "", "", true
}

// matchArray treats the expected array as a positional prefix of the actual array.
func matchArray(actual any, expected []any, path string) (mismatchPath, mismatchReason string, matched bool) {
	actualSlice, ok := actual.([]any)
	if !ok {
		return path, "actual value is not array", false
	}
	if len(actualSlice) < len(expected) {
		reason := fmt.Sprintf(
			"actual array length %d is less than expected %d",
			len(actualSlice),
			len(expected),
		)
		return path, reason, false
	}

	for index, expectedValue := range expected {
		nestedPath, nestedReason, nestedMatched := Match(
			actualSlice[index],
			expectedValue,
			fmt.Sprintf("%s[%d]", path, index),
		)
		if !nestedMatched {
			return nestedPath, nestedReason, false
		}
	}

	return "", "", true
}

// matchScalar requires exact decoded-value equality for non-object and non-array values.
func matchScalar(actual, expected any, path string) (mismatchPath, mismatchReason string, matched bool) {
	if reflect.DeepEqual(actual, expected) {
		return "", "", true
	}
	return path, fmt.Sprintf("expected %v, got %v", expected, actual), false
}
