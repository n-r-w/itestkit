package itestkit

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"go.uber.org/mock/gomock"
)

// newMockHandler creates a gomock handler with a decoding setting.
func newMockHandler(t *testing.T, ctrl *gomock.Controller) *MockHandler[any] {
	t.Helper()

	handler := NewMockHandler[any](ctrl)
	handler.EXPECT().DecodeRequest(gomock.Any()).AnyTimes().DoAndReturn(
		func(raw json.RawMessage) (any, error) {
			return decodeJSON(raw)
		},
	)
	handler.EXPECT().DecodeExpectedResponse(gomock.Any()).AnyTimes().DoAndReturn(
		func(raw json.RawMessage) (any, error) {
			return decodeJSON(raw)
		},
	)

	return handler
}

// newMockCaseSource creates a gomock case source on top of MapFS.
func newMockCaseSource(t *testing.T, ctrl *gomock.Controller, source fstest.MapFS) *MockCaseSource {
	t.Helper()

	mockSource := NewMockCaseSource(ctrl)
	mockSource.EXPECT().ReadDir(gomock.Any()).AnyTimes().DoAndReturn(
		func(name string) ([]fs.DirEntry, error) {
			return source.ReadDir(name)
		},
	)
	mockSource.EXPECT().ReadFile(gomock.Any()).AnyTimes().DoAndReturn(
		func(name string) ([]byte, error) {
			return source.ReadFile(name)
		},
	)

	return mockSource
}

// newMockRegistry creates a gomock registry of handlers with the given names.
func newMockRegistry(t *testing.T, ctrl *gomock.Controller, handlers map[string]Handler[any]) *MockHandlerRegistry[any] {
	t.Helper()

	registry := NewMockHandlerRegistry[any](ctrl)
	registry.EXPECT().Resolve(gomock.Any()).AnyTimes().DoAndReturn(
		func(name string) (Handler[any], error) {
			handler, exists := handlers[name]
			if !exists {
				return nil, errors.New("handler not found")
			}
			return handler, nil
		},
	)
	registry.EXPECT().Supported().AnyTimes().Return(sortedHandlerNames(handlers))

	return registry
}

// newMockStatusCodec creates a gomock status codec with parsing test codes.
func newMockStatusCodec(t *testing.T, ctrl *gomock.Controller) *MockStatusCodec[testStatus] {
	t.Helper()

	codec := NewMockStatusCodec[testStatus](ctrl)
	codec.EXPECT().Parse(gomock.Any()).AnyTimes().DoAndReturn(
		func(raw string) (testStatus, error) {
			return parseTestStatus(raw)
		},
	)
	codec.EXPECT().Success().AnyTimes().Return(testStatusOK)

	return codec
}

// sortedHandlerNames returns sorted handler names.
func sortedHandlerNames(handlers map[string]Handler[any]) []string {
	if len(handlers) == 0 {
		return nil
	}

	result := make([]string, 0, len(handlers))
	for name := range handlers {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

// testStatus specifies the status for kernel tests.
type testStatus string

const (
	testStatusOK     testStatus = "OK"
	testStatusFailed testStatus = "ERROR"
)

// parseTestStatus converts string code to testStatus.
func parseTestStatus(raw string) (testStatus, error) {
	normalized := strings.ToUpper(strings.TrimSpace(raw))
	switch normalized {
	case string(testStatusOK):
		return testStatusOK, nil
	case string(testStatusFailed):
		return testStatusFailed, nil
	default:
		return testStatusFailed, fmt.Errorf("unsupported status: %q", raw)
	}
}
