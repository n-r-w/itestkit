package itestkit

import (
	"encoding/json"
	"fmt"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestLoadCases_RecursesAndSorts tests path recursion and sorting.
func TestLoadCases_RecursesAndSorts(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	handler := newMockHandler(t, ctrl)
	registry := newMockRegistry(t, ctrl, map[string]Handler[any]{
		"Handler": handler,
	})
	statusCodec := newMockStatusCodec(t, ctrl)

	fs := fstest.MapFS{
		"cases/a.jsonc":          {Data: []byte(makeCaseJSONC("case-a", "Handler"))},
		"cases/nested/c.jsonc":   {Data: []byte(makeCaseJSONC("case-c", "Handler"))},
		"cases/b.jsonc":          {Data: []byte(makeCaseJSONC("case-b", "Handler"))},
		"cases/notes.txt":        {Data: []byte("ignore")},
		"cases/nested/extra.txt": {Data: []byte("ignore")},
	}
	source := newMockCaseSource(t, ctrl, fs)

	cases, err := LoadCases(source, "cases", registry, statusCodec)
	require.NoError(t, err)
	require.Len(t, cases, 3)
	require.Equal(
		t,
		[]string{"cases/a.jsonc", "cases/b.jsonc", "cases/nested/c.jsonc"},
		[]string{cases[0].SourcePath, cases[1].SourcePath, cases[2].SourcePath},
	)
}

// TestLoadCases_EmptyDirectory checks for an error if there are no jsonc files.
func TestLoadCases_EmptyDirectory(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	registry := newMockRegistry(t, ctrl, nil)
	statusCodec := newMockStatusCodec(t, ctrl)
	fs := fstest.MapFS{
		"cases/.keep": {Data: []byte("ignore")},
	}
	source := newMockCaseSource(t, ctrl, fs)

	cases, err := LoadCases(source, "cases", registry, statusCodec)
	require.Error(t, err)
	require.Nil(t, cases)
	require.Contains(t, err.Error(), "no .jsonc cases found")
}

// TestLoadCases_MinimalValidCase checks the minimum valid case of the new format.
func TestLoadCases_MinimalValidCase(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	registry := makeRegistryWithHandler(t, ctrl, "Handler", newMockHandler(t, ctrl))
	statusCodec := newMockStatusCodec(t, ctrl)
	fs := fstest.MapFS{
		"cases/case.jsonc": {Data: []byte(makeCaseJSONC("minimal", "Handler"))},
	}
	source := newMockCaseSource(t, ctrl, fs)

	cases, err := LoadCases(source, "cases", registry, statusCodec)
	require.NoError(t, err)
	require.Len(t, cases, 1)
	require.Equal(t, "minimal", cases[0].Name)
	require.Len(t, cases[0].Steps, 1)
	require.Equal(t, StepKindAction, cases[0].Steps[0].Kind)
	require.Equal(t, "action-1", cases[0].Steps[0].ID)
	require.Equal(t, map[string]any{"ok": true}, cases[0].Assert.Response)
}

// TestLoadCases_UnknownField checks for strong JSON validation.
func TestLoadCases_UnknownField(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	registry := makeRegistryWithHandler(t, ctrl, "Handler", newMockHandler(t, ctrl))
	statusCodec := newMockStatusCodec(t, ctrl)
	caseJSON := `{
		"name":"case",
		"steps":[{"id":"action-1","kind":"action","handler":"Handler","request":{"id":1}}],
		"assert":{"code":"OK","response":{"ok":true}},
		"extra":true
	}`
	fs := fstest.MapFS{
		"cases/case.jsonc": {Data: []byte(caseJSON)},
	}
	source := newMockCaseSource(t, ctrl, fs)

	_, err := LoadCases(source, "cases", registry, statusCodec)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown field")
}

// TestLoadCases_TrailingJSON tests trailing JSON detection.
func TestLoadCases_TrailingJSON(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	registry := makeRegistryWithHandler(t, ctrl, "Handler", newMockHandler(t, ctrl))
	statusCodec := newMockStatusCodec(t, ctrl)
	caseJSON := makeCaseJSONC("case", "Handler") + " {}"
	fs := fstest.MapFS{
		"cases/case.jsonc": {Data: []byte(caseJSON)},
	}
	source := newMockCaseSource(t, ctrl, fs)

	_, err := LoadCases(source, "cases", registry, statusCodec)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected trailing JSON")
}

// TestLoadCases_DuplicateName checks the detection of duplicate names.
func TestLoadCases_DuplicateName(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	registry := makeRegistryWithHandler(t, ctrl, "Handler", newMockHandler(t, ctrl))
	statusCodec := newMockStatusCodec(t, ctrl)
	fs := fstest.MapFS{
		"cases/one.jsonc": {Data: []byte(makeCaseJSONC("dup", "Handler"))},
		"cases/two.jsonc": {Data: []byte(makeCaseJSONC("dup", "Handler"))},
	}
	source := newMockCaseSource(t, ctrl, fs)

	_, err := LoadCases(source, "cases", registry, statusCodec)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate case name")
}

// TestLoadCases_MissingSteps checks whether steps are required.
func TestLoadCases_MissingSteps(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	registry := makeRegistryWithHandler(t, ctrl, "Handler", newMockHandler(t, ctrl))
	statusCodec := newMockStatusCodec(t, ctrl)
	caseJSON := `{
		"name":"case",
		"steps":[],
		"assert":{"code":"OK","response":{"ok":true}}
	}`
	fs := fstest.MapFS{
		"cases/case.jsonc": {Data: []byte(caseJSON)},
	}
	source := newMockCaseSource(t, ctrl, fs)

	_, err := LoadCases(source, "cases", registry, statusCodec)
	require.Error(t, err)
	require.Contains(t, err.Error(), "field steps is required")
}

// TestLoadCases_UnknownHandlerIncludesSupported tests the handler error message.
func TestLoadCases_UnknownHandlerIncludesSupported(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	registry := makeRegistryWithHandler(t, ctrl, "Handler", newMockHandler(t, ctrl))
	statusCodec := newMockStatusCodec(t, ctrl)
	caseJSON := makeCaseJSONC("case", "Unknown")
	fs := fstest.MapFS{
		"cases/case.jsonc": {Data: []byte(caseJSON)},
	}
	source := newMockCaseSource(t, ctrl, fs)

	_, err := LoadCases(source, "cases", registry, statusCodec)
	require.Error(t, err)
	require.Contains(t, err.Error(), "supported handlers")
	require.Contains(t, err.Error(), "Handler")
}

// TestLoadCases_DuplicateStepID checks the uniqueness of step.id within a case.
func TestLoadCases_DuplicateStepID(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	registry := makeRegistryWithHandler(t, ctrl, "Handler", newMockHandler(t, ctrl))
	statusCodec := newMockStatusCodec(t, ctrl)
	caseJSON := `{
		"name":"case",
		"steps":[
			{"id":"dup","kind":"prepare","handler":"Handler","request":{"id":1}},
			{"id":"dup","kind":"action","handler":"Handler","request":{"id":2}}
		],
		"assert":{"code":"OK","response":{"ok":true}}
	}`
	fs := fstest.MapFS{
		"cases/case.jsonc": {Data: []byte(caseJSON)},
	}
	source := newMockCaseSource(t, ctrl, fs)

	_, err := LoadCases(source, "cases", registry, statusCodec)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate step id \"dup\"")
}

// TestLoadCases_UnknownStepKind checks the validation of valid step.kind values.
func TestLoadCases_UnknownStepKind(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	registry := makeRegistryWithHandler(t, ctrl, "Handler", newMockHandler(t, ctrl))
	statusCodec := newMockStatusCodec(t, ctrl)
	caseJSON := `{
		"name":"case",
		"steps":[{"id":"s1","kind":"invalid","handler":"Handler","request":{"id":1}}],
		"assert":{"code":"OK","response":{"ok":true}}
	}`
	fs := fstest.MapFS{
		"cases/case.jsonc": {Data: []byte(caseJSON)},
	}
	source := newMockCaseSource(t, ctrl, fs)

	_, err := LoadCases(source, "cases", registry, statusCodec)
	require.Error(t, err)
	require.Contains(t, err.Error(), "field steps[0].kind: unsupported kind \"invalid\"")
	require.Contains(t, err.Error(), "allowed kinds: prepare, action, publish, await, verify, cleanup")
}

// TestLoadCases_AwaitRetryRequired checks whether retry is required for kind=await.
func TestLoadCases_AwaitRetryRequired(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	registry := makeRegistryWithHandler(t, ctrl, "Handler", newMockHandler(t, ctrl))
	statusCodec := newMockStatusCodec(t, ctrl)
	caseJSON := `{
		"name":"case",
		"steps":[
			{"id":"await-1","kind":"await","handler":"Handler","request":{"id":1}},
			{"id":"action-1","kind":"action","handler":"Handler","request":{"id":2}}
		],
		"assert":{"code":"OK","response":{"ok":true}}
	}`
	fs := fstest.MapFS{
		"cases/case.jsonc": {Data: []byte(caseJSON)},
	}
	source := newMockCaseSource(t, ctrl, fs)

	_, err := LoadCases(source, "cases", registry, statusCodec)
	require.Error(t, err)
	require.Contains(t, err.Error(), "field steps[0].retry is required for kind=await")
}

// TestLoadCases_AwaitRetryInvalid checks for strict validation of retry fields.
func TestLoadCases_AwaitRetryInvalid(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	registry := makeRegistryWithHandler(t, ctrl, "Handler", newMockHandler(t, ctrl))
	statusCodec := newMockStatusCodec(t, ctrl)
	caseJSON := `{
		"name":"case",
		"steps":[
			{
				"id":"await-1",
				"kind":"await",
				"handler":"Handler",
				"request":{"id":1},
				"retry":{"timeout_ms":0,"interval_ms":10,"max_attempts":3}
			},
			{"id":"action-1","kind":"action","handler":"Handler","request":{"id":2}}
		],
		"assert":{"code":"OK","response":{"ok":true}}
	}`
	fs := fstest.MapFS{
		"cases/case.jsonc": {Data: []byte(caseJSON)},
	}
	source := newMockCaseSource(t, ctrl, fs)

	_, err := LoadCases(source, "cases", registry, statusCodec)
	require.Error(t, err)
	require.Contains(t, err.Error(), "field steps[0].retry.timeout_ms must be > 0")
}

// TestLoadCases_InvalidAssertResponseFromStep checks the referential integrity of assert.response_from_step.
func TestLoadCases_InvalidAssertResponseFromStep(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	registry := makeRegistryWithHandler(t, ctrl, "Handler", newMockHandler(t, ctrl))
	statusCodec := newMockStatusCodec(t, ctrl)
	caseJSON := `{
		"name":"case",
		"steps":[{"id":"action-1","kind":"action","handler":"Handler","request":{"id":1}}],
		"assert":{"code":"OK","response":{"ok":true},"response_from_step":"missing"}
	}`
	fs := fstest.MapFS{
		"cases/case.jsonc": {Data: []byte(caseJSON)},
	}
	source := newMockCaseSource(t, ctrl, fs)

	_, err := LoadCases(source, "cases", registry, statusCodec)
	require.Error(t, err)
	require.Contains(t, err.Error(), "field assert.response_from_step: unknown step id \"missing\"")
}

// TestLoadCases_ResponseWithoutActionStep checks the default assert.response targeting.
func TestLoadCases_ResponseWithoutActionStep(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	registry := makeRegistryWithHandler(t, ctrl, "Handler", newMockHandler(t, ctrl))
	statusCodec := newMockStatusCodec(t, ctrl)
	caseJSON := `{
		"name":"case",
		"steps":[{"id":"prepare-1","kind":"prepare","handler":"Handler","request":{"id":1}}],
		"assert":{"code":"OK","response":{"ok":true}}
	}`
	fs := fstest.MapFS{
		"cases/case.jsonc": {Data: []byte(caseJSON)},
	}
	source := newMockCaseSource(t, ctrl, fs)

	_, err := LoadCases(source, "cases", registry, statusCodec)
	require.Error(t, err)
	require.Contains(t, err.Error(), "field assert.response requires at least one action step")
}

// TestLoadCases_DefaultAssertResponseFromLastAction checks the selection of the last default action.
func TestLoadCases_DefaultAssertResponseFromLastAction(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	firstHandler := NewMockHandler[any](ctrl)
	firstHandler.EXPECT().DecodeRequest(gomock.Any()).Times(1).DoAndReturn(
		func(raw json.RawMessage) (any, error) {
			return decodeJSON(raw)
		},
	)
	firstHandler.EXPECT().DecodeExpectedResponse(gomock.Any()).Times(0)

	lastHandler := NewMockHandler[any](ctrl)
	lastHandler.EXPECT().DecodeRequest(gomock.Any()).Times(1).DoAndReturn(
		func(raw json.RawMessage) (any, error) {
			return decodeJSON(raw)
		},
	)
	lastHandler.EXPECT().DecodeExpectedResponse(gomock.Any()).Times(1).DoAndReturn(
		func(raw json.RawMessage) (any, error) {
			return decodeJSON(raw)
		},
	)

	registry := newMockRegistry(t, ctrl, map[string]Handler[any]{
		"First": firstHandler,
		"Last":  lastHandler,
	})
	statusCodec := newMockStatusCodec(t, ctrl)

	caseJSON := `{
		"name":"case",
		"steps":[
			{"id":"action-1","kind":"action","handler":"First","request":{"value":1}},
			{"id":"action-2","kind":"action","handler":"Last","request":{"value":2}}
		],
		"assert":{"code":"OK","response":{"selected":"last"}}
	}`
	fs := fstest.MapFS{
		"cases/case.jsonc": {Data: []byte(caseJSON)},
	}
	source := newMockCaseSource(t, ctrl, fs)

	cases, err := LoadCases(source, "cases", registry, statusCodec)
	require.NoError(t, err)
	require.Len(t, cases, 1)
	require.Empty(t, cases[0].Assert.ResponseFromStep)
	require.Equal(t, map[string]any{"selected": "last"}, cases[0].Assert.Response)
}

// TestLoadCases_PartialAssertResponseKeepsRawJSON checks that the mode is `partial`
// saves the JSON form of the fixture and does not pass through the handler decoder.
func TestLoadCases_PartialAssertResponseKeepsRawJSON(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	handler := NewMockHandler[any](ctrl)
	handler.EXPECT().DecodeRequest(gomock.Any()).Times(1).DoAndReturn(
		func(raw json.RawMessage) (any, error) {
			return decodeJSON(raw)
		},
	)
	handler.EXPECT().DecodeExpectedResponse(gomock.Any()).Times(0)

	registry := makeRegistryWithHandler(t, ctrl, "Handler", handler)
	statusCodec := newMockStatusCodec(t, ctrl)

	caseJSON := `{
		"name":"case",
		"steps":[
			{"id":"action-1","kind":"action","handler":"Handler","request":{"value":1}}
		],
		"assert":{
			"code":"OK",
			"response_mode":"partial",
			"response":{"status":"SERVING","nested":{"value":1}}
		}
	}`
	fs := fstest.MapFS{
		"cases/case.jsonc": {Data: []byte(caseJSON)},
	}
	source := newMockCaseSource(t, ctrl, fs)

	cases, err := LoadCases(source, "cases", registry, statusCodec)
	require.NoError(t, err)
	require.Len(t, cases, 1)
	require.Equal(t, ResponseModePartial, cases[0].Assert.ResponseMode)
	require.Equal(t, map[string]any{
		"status": "SERVING",
		"nested": map[string]any{
			"value": json.Number("1"),
		},
	}, cases[0].Assert.Response)
}

// TestLoadCases_ExplicitExactAssertResponseUsesDecoder checks that explicit mode is `exact`
// stores the current decoding path of the expected response through the handler.
func TestLoadCases_ExplicitExactAssertResponseUsesDecoder(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	handler := NewMockHandler[any](ctrl)
	handler.EXPECT().DecodeRequest(gomock.Any()).Times(1).DoAndReturn(
		func(raw json.RawMessage) (any, error) {
			return decodeJSON(raw)
		},
	)
	handler.EXPECT().DecodeExpectedResponse(gomock.Any()).Times(1).Return("decoded-response", nil)

	registry := makeRegistryWithHandler(t, ctrl, "Handler", handler)
	statusCodec := newMockStatusCodec(t, ctrl)

	caseJSON := `{
		"name":"case",
		"steps":[
			{"id":"action-1","kind":"action","handler":"Handler","request":{"value":1}}
		],
		"assert":{
			"code":"OK",
			"response_mode":"exact",
			"response":{"status":"SERVING"}
		}
	}`
	fs := fstest.MapFS{
		"cases/case.jsonc": {Data: []byte(caseJSON)},
	}
	source := newMockCaseSource(t, ctrl, fs)

	cases, err := LoadCases(source, "cases", registry, statusCodec)
	require.NoError(t, err)
	require.Len(t, cases, 1)
	require.Equal(t, ResponseModeExact, cases[0].Assert.ResponseMode)
	require.Equal(t, "decoded-response", cases[0].Assert.Response)
}

// TestLoadCases_ExactAssertResponseWithPresentMarkerKeepsRawJSON checks that marker fields are not decoded before runtime.
func TestLoadCases_ExactAssertResponseWithPresentMarkerKeepsRawJSON(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	handler := NewMockHandler[any](ctrl)
	handler.EXPECT().DecodeRequest(gomock.Any()).Times(1).DoAndReturn(
		func(raw json.RawMessage) (any, error) {
			return decodeJSON(raw)
		},
	)
	handler.EXPECT().DecodeExpectedResponse(gomock.Any()).Times(0)

	registry := makeRegistryWithHandler(t, ctrl, "Handler", handler)
	statusCodec := newMockStatusCodec(t, ctrl)

	caseJSON := `{
		"name":"case",
		"steps":[
			{"id":"action-1","kind":"action","handler":"Handler","request":{"value":1}}
		],
		"assert":{
			"code":"OK",
			"response_mode":"exact",
			"response":{"id":"<itestkit_present>","version":"<itestkit_present>","status":"SERVING"}
		}
	}`
	fs := fstest.MapFS{
		"cases/case.jsonc": {Data: []byte(caseJSON)},
	}
	source := newMockCaseSource(t, ctrl, fs)

	cases, err := LoadCases(source, "cases", registry, statusCodec)
	require.NoError(t, err)
	require.Len(t, cases, 1)
	require.Equal(t, ResponseModeExact, cases[0].Assert.ResponseMode)
	require.Equal(t, map[string]any{
		"id":      responsePresentMarker,
		"version": responsePresentMarker,
		"status":  "SERVING",
	}, cases[0].Assert.Response)
}

// TestLoadCases_InvalidAssertResponseMode tests the validation of an unknown response mode.
func TestLoadCases_InvalidAssertResponseMode(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	handler := newMockHandler(t, ctrl)
	registry := makeRegistryWithHandler(t, ctrl, "Handler", handler)
	statusCodec := newMockStatusCodec(t, ctrl)

	caseJSON := `{
		"name":"case",
		"steps":[
			{"id":"action-1","kind":"action","handler":"Handler","request":{"value":1}}
		],
		"assert":{"code":"OK","response_mode":"subset","response":{"ok":true}}
	}`
	fs := fstest.MapFS{
		"cases/case.jsonc": {Data: []byte(caseJSON)},
	}
	source := newMockCaseSource(t, ctrl, fs)

	_, err := LoadCases(source, "cases", registry, statusCodec)
	require.Error(t, err)
	require.Contains(t, err.Error(), "field assert.response_mode")
	require.Contains(t, err.Error(), "subset")
}

// TestLoadCases_PartialAssertResponseModeRequiresSuccess checks whether the `partial` mode is disabled
// for scenarios where the successful response is not compared.
func TestLoadCases_PartialAssertResponseModeRequiresSuccess(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	handler := newMockHandler(t, ctrl)
	registry := makeRegistryWithHandler(t, ctrl, "Handler", handler)
	statusCodec := newMockStatusCodec(t, ctrl)

	caseJSON := `{
		"name":"case",
		"steps":[
			{"id":"action-1","kind":"action","handler":"Handler","request":{"value":1}}
		],
		"assert":{"code":"ERROR","response_mode":"partial","message_contains":"failed"}
	}`
	fs := fstest.MapFS{
		"cases/case.jsonc": {Data: []byte(caseJSON)},
	}
	source := newMockCaseSource(t, ctrl, fs)

	_, err := LoadCases(source, "cases", registry, statusCodec)
	require.Error(t, err)
	require.Contains(t, err.Error(), "field assert.response_mode")
	require.Contains(t, err.Error(), "requires code=OK")
}

// TestLoadCases_DynamicRequestTemplateDeferredDecode checks the deferred decode for requests with templates.
func TestLoadCases_DynamicRequestTemplateDeferredDecode(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	sourceHandler := NewMockHandler[any](ctrl)
	sourceHandler.EXPECT().DecodeRequest(gomock.Any()).Times(1).DoAndReturn(
		func(raw json.RawMessage) (any, error) {
			return decodeJSON(raw)
		},
	)
	sourceHandler.EXPECT().DecodeExpectedResponse(gomock.Any()).Times(0)

	dynamicHandler := NewMockHandler[any](ctrl)
	dynamicHandler.EXPECT().DecodeRequest(gomock.Any()).Times(0)
	dynamicHandler.EXPECT().DecodeExpectedResponse(gomock.Any()).Times(0)

	registry := newMockRegistry(t, ctrl, map[string]Handler[any]{
		"Source":  sourceHandler,
		"Dynamic": dynamicHandler,
	})
	statusCodec := newMockStatusCodec(t, ctrl)

	caseJSON := `{
		"name":"case",
		"steps":[
			{"id":"action-1","kind":"action","handler":"Source","request":{"value":"order-100"}},
			{"id":"verify-1","kind":"verify","handler":"Dynamic","request":{"order_id":"{{steps.action-1.response.value}}"}}
		],
		"assert":{"code":"ERROR","message_contains":"deferred decode"}
	}`
	fs := fstest.MapFS{
		"cases/case.jsonc": {Data: []byte(caseJSON)},
	}
	source := newMockCaseSource(t, ctrl, fs)

	cases, err := LoadCases(source, "cases", registry, statusCodec)
	require.NoError(t, err)
	require.Len(t, cases, 1)
	require.Len(t, cases[0].Steps, 2)

	_, isDynamicRequest := cases[0].Steps[1].Request.(stepDynamicRequest)
	require.True(t, isDynamicRequest)
}

// makeRegistryWithHandler creates a registry with one test handle.
func makeRegistryWithHandler(t *testing.T, ctrl *gomock.Controller, name string, handler Handler[any]) *MockHandlerRegistry[any] {
	t.Helper()

	return newMockRegistry(t, ctrl, map[string]Handler[any]{
		name: handler,
	})
}

// makeCaseJSONC generates a minimally valid JSONC case.
func makeCaseJSONC(name string, handler string) string {
	return fmt.Sprintf(`{
		// case name
		"name":"%s",
		"labels":["smoke"],
		"description":"loader case",
		"steps":[
			{
				"id":"action-1",
				"kind":"action",
				"handler":"%s",
				"request":{"id":1}
			}
		],
		"assert":{
			"code":"OK",
			"message_contains":"",
			"response":{"ok":true}
		}
	}`, name, handler)
}

// decodeJSON decodes JSON into an arbitrary value for tests.
func decodeJSON(raw json.RawMessage) (any, error) {
	var result any
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}
