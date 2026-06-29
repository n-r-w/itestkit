package itestkit

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// runnerEqualMethodValue checks that value equality without protobuf does not pass to cmp.Equal.
type runnerEqualMethodValue struct {
	Value string
}

// Equal intentionally hides the difference in Value under cmp.Equal.
func (runnerEqualMethodValue) Equal(runnerEqualMethodValue) bool {
	return true
}

// runnerProtoStructValue checks a structure with a protobuf field and non-exportable state.
type runnerProtoStructValue struct {
	Response *wrapperspb.BytesValue
	hidden   string
}

// runnerPresentMarkerStructResponse models a normalized response that keeps typed fields with JSON names.
type runnerPresentMarkerStructResponse struct {
	OrderID string `json:"order_id"`
	Amount  int    `json:"amount"`
	Stored  bool   `json:"stored"`
}

// TestRunCases_OrderAndDefaultActionAssertTarget checks the order of steps and the default assert target (last action).
func TestRunCases_OrderAndDefaultActionAssertTarget(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	handler := newMockHandler(t, ctrl)
	statusCodec := newMockStatusCodec(t, ctrl)
	factory := NewMockHarnessFactory[any](ctrl)
	factory.EXPECT().New(gomock.Any()).Times(1).Return(nil)
	inspector := NewMockErrorInspector[testStatus](ctrl)

	callOrder := make([]string, 0, 5)
	handler.EXPECT().Invoke(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
		func(_ context.Context, _ any, request any) (any, error) {
			label, ok := request.(string)
			if !ok {
				return nil, errors.New("request is not a string")
			}
			callOrder = append(callOrder, label)

			switch label {
			case "prepare":
				return "prepare-out", nil
			case "action-1":
				return "action-1-out", nil
			case "verify":
				return "verify-out", nil
			case "action-2":
				return "action-2-out", nil
			case "cleanup":
				return "cleanup-out", nil
			default:
				return nil, errors.New("unexpected request")
			}
		},
	)
	handler.EXPECT().NormalizeResponse(gomock.Any()).AnyTimes().DoAndReturn(
		func(response any) (any, error) {
			return response, nil
		},
	)

	testCase := Case[any, testStatus]{
		Name:       "case",
		SourcePath: "cases/case.jsonc",
		Steps: []Step[any]{
			newRunnerStep("s-prepare", StepKindPrepare, "Handler", handler, "prepare", nil),
			newRunnerStep("s-action-1", StepKindAction, "Handler", handler, "action-1", nil),
			newRunnerStep("s-verify", StepKindVerify, "Handler", handler, "verify", nil),
			newRunnerStep("s-action-2", StepKindAction, "Handler", handler, "action-2", nil),
			newRunnerStep("s-cleanup", StepKindCleanup, "Handler", handler, "cleanup", nil),
		},
		Assert: newRunnerAssert(testStatusOK, "", "action-2-out", ""),
	}

	err := executeCases(
		t,
		[]Case[any, testStatus]{testCase},
		factory,
		inspector,
		statusCodec,
		defaultRunCasesConfig(),
	)
	require.NoError(t, err)
	require.Equal(t, []string{"prepare", "action-1", "verify", "action-2", "cleanup"}, callOrder)
}

// TestRunCases_AssertResponseFromStep checks the selection of an assert target by response_from_step.
func TestRunCases_AssertResponseFromStep(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	handler := newMockHandler(t, ctrl)
	statusCodec := newMockStatusCodec(t, ctrl)
	factory := NewMockHarnessFactory[any](ctrl)
	factory.EXPECT().New(gomock.Any()).Times(1).Return(nil)
	inspector := NewMockErrorInspector[testStatus](ctrl)

	handler.EXPECT().Invoke(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
		func(_ context.Context, _ any, request any) (any, error) {
			label, ok := request.(string)
			if !ok {
				return nil, errors.New("request is not a string")
			}
			if label == "first" {
				return "first-out", nil
			}
			if label == "last" {
				return "last-out", nil
			}

			return nil, errors.New("unexpected request")
		},
	)
	handler.EXPECT().NormalizeResponse(gomock.Any()).AnyTimes().DoAndReturn(
		func(response any) (any, error) {
			return response, nil
		},
	)

	testCase := Case[any, testStatus]{
		Name:       "case",
		SourcePath: "cases/case.jsonc",
		Steps: []Step[any]{
			newRunnerStep("a-1", StepKindAction, "Handler", handler, "first", nil),
			newRunnerStep("a-2", StepKindAction, "Handler", handler, "last", nil),
		},
		Assert: newRunnerAssert(testStatusOK, "", "first-out", "a-1"),
	}

	err := executeCases(
		t,
		[]Case[any, testStatus]{testCase},
		factory,
		inspector,
		statusCodec,
		defaultRunCasesConfig(),
	)
	require.NoError(t, err)
}

// TestRunCases_AwaitRetryUntilSuccess tests the retry behavior of the await step until it succeeds.
func TestRunCases_AwaitRetryUntilSuccess(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	handler := newMockHandler(t, ctrl)
	statusCodec := newMockStatusCodec(t, ctrl)
	factory := NewMockHarnessFactory[any](ctrl)
	factory.EXPECT().New(gomock.Any()).Times(1).Return(nil)
	inspector := NewMockErrorInspector[testStatus](ctrl)

	attempts := 0
	handler.EXPECT().Invoke(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
		func(_ context.Context, _ any, _ any) (any, error) {
			attempts++
			if attempts < 3 {
				return "pending", errors.New("not ready")
			}

			return "ready", nil
		},
	)
	handler.EXPECT().NormalizeResponse(gomock.Any()).AnyTimes().DoAndReturn(
		func(response any) (any, error) {
			return response, nil
		},
	)

	testCase := Case[any, testStatus]{
		Name:       "case",
		SourcePath: "cases/case.jsonc",
		Steps: []Step[any]{
			newRunnerStep(
				"await-1",
				StepKindAwait,
				"Await",
				handler,
				"await-request",
				&AwaitRetry{TimeoutMS: 300, IntervalMS: 1, MaxAttempts: 5},
			),
		},
		Assert: newRunnerAssert(testStatusOK, "", "ready", "await-1"),
	}

	err := executeCases(
		t,
		[]Case[any, testStatus]{testCase},
		factory,
		inspector,
		statusCodec,
		defaultRunCasesConfig(),
	)
	require.NoError(t, err)
	require.Equal(t, 3, attempts)
}

// TestRunCases_AwaitRetryDeadlineAllowsFinalInvoke checks the final call to the await handler
// after the deadline has passed to allow the zero-count await to succeed gracefully.
func TestRunCases_AwaitRetryDeadlineAllowsFinalInvoke(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	handler := newMockHandler(t, ctrl)
	statusCodec := newMockStatusCodec(t, ctrl)
	factory := NewMockHarnessFactory[any](ctrl)
	factory.EXPECT().New(gomock.Any()).Times(1).Return(nil)
	inspector := NewMockErrorInspector[testStatus](ctrl)

	attempts := 0
	handler.EXPECT().Invoke(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
		func(ctx context.Context, _ any, _ any) (any, error) {
			attempts++
			select {
			case <-ctx.Done():
				return "window-complete", nil
			default:
			}

			return "pending", errors.New("await retry window is still open")
		},
	)
	handler.EXPECT().NormalizeResponse(gomock.Any()).AnyTimes().DoAndReturn(
		func(response any) (any, error) {
			return response, nil
		},
	)

	testCase := Case[any, testStatus]{
		Name:       "case",
		SourcePath: "cases/case.jsonc",
		Steps: []Step[any]{
			newRunnerStep(
				"await-1",
				StepKindAwait,
				"Await",
				handler,
				"await-request",
				&AwaitRetry{TimeoutMS: 20, IntervalMS: 200, MaxAttempts: 5},
			),
		},
		Assert: newRunnerAssert(testStatusOK, "", "window-complete", "await-1"),
	}

	err := executeCases(
		t,
		[]Case[any, testStatus]{testCase},
		factory,
		inspector,
		statusCodec,
		defaultRunCasesConfig(),
	)
	require.NoError(t, err)
	require.Equal(t, 2, attempts)
}

// TestRunCases_PublishAwaitVerifyFlow checks the publish/await/verify scenario on a single step-pipeline.
func TestRunCases_PublishAwaitVerifyFlow(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	handler := newMockHandler(t, ctrl)
	statusCodec := newMockStatusCodec(t, ctrl)
	factory := NewMockHarnessFactory[any](ctrl)
	factory.EXPECT().New(gomock.Any()).Times(1).Return(nil)
	inspector := NewMockErrorInspector[testStatus](ctrl)

	awaitAttempts := 0
	callOrder := make([]string, 0, 4)
	handler.EXPECT().Invoke(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
		func(_ context.Context, _ any, request any) (any, error) {
			label, ok := request.(string)
			if !ok {
				return nil, errors.New("request is not a string")
			}

			callOrder = append(callOrder, label)
			switch label {
			case "publish":
				return "event-id-1", nil
			case "await":
				awaitAttempts++
				if awaitAttempts < 2 {
					return "pending", errors.New("not processed yet")
				}
				return "processed", nil
			case "verify":
				return "state=processed", nil
			default:
				return nil, errors.New("unexpected request")
			}
		},
	)
	handler.EXPECT().NormalizeResponse(gomock.Any()).AnyTimes().DoAndReturn(
		func(response any) (any, error) {
			return response, nil
		},
	)

	testCase := Case[any, testStatus]{
		Name:       "publish-await-verify",
		SourcePath: "cases/publish-await-verify.jsonc",
		Steps: []Step[any]{
			newRunnerStep("publish-1", StepKindPublish, "Handler", handler, "publish", nil),
			newRunnerStep(
				"await-1",
				StepKindAwait,
				"Handler",
				handler,
				"await",
				&AwaitRetry{TimeoutMS: 300, IntervalMS: 1, MaxAttempts: 5},
			),
			newRunnerStep("verify-1", StepKindVerify, "Handler", handler, "verify", nil),
		},
		Assert: newRunnerAssert(testStatusOK, "", "state=processed", "verify-1"),
	}

	err := executeCases(
		t,
		[]Case[any, testStatus]{testCase},
		factory,
		inspector,
		statusCodec,
		defaultRunCasesConfig(),
	)
	require.NoError(t, err)
	require.Equal(t, []string{"publish", "await", "await", "verify"}, callOrder)
}

// TestRunCases_AwaitRetryExhaustedIncludesPolicyAndState checks await diagnostics when retry is exhausted.
func TestRunCases_AwaitRetryExhaustedIncludesPolicyAndState(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	handler := newMockHandler(t, ctrl)
	statusCodec := newMockStatusCodec(t, ctrl)
	factory := NewMockHarnessFactory[any](ctrl)
	factory.EXPECT().New(gomock.Any()).Times(1).Return(nil)
	inspector := NewMockErrorInspector[testStatus](ctrl)

	handler.EXPECT().Invoke(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
		func(_ context.Context, _ any, _ any) (any, error) {
			return "pending-state", errors.New("still pending")
		},
	)

	testCase := Case[any, testStatus]{
		Name:       "case",
		SourcePath: "cases/case.jsonc",
		Steps: []Step[any]{
			newRunnerStep(
				"await-1",
				StepKindAwait,
				"Await",
				handler,
				"await-request",
				&AwaitRetry{TimeoutMS: 20, IntervalMS: 1, MaxAttempts: 2},
			),
		},
		Assert: newRunnerAssert(testStatusOK, "", nil, ""),
	}

	err := runCase(t.Context(), t, testCase, factory, inspector, statusCodec, defaultRunCasesConfig())
	require.Error(t, err)
	require.Contains(t, err.Error(), "step.id=await-1")
	require.Contains(t, err.Error(), "kind=await")
	require.Contains(t, err.Error(), "handler=Await")
	require.Contains(t, err.Error(), "timeout_ms=20")
	require.Contains(t, err.Error(), "interval_ms=1")
	require.Contains(t, err.Error(), "max_attempts=2")
	require.Contains(t, err.Error(), "last_error=still pending")
	require.Contains(t, err.Error(), "last_response=pending-state")
}

// TestRunCases_CleanupRunsOnFailure tests deterministic cleanup semantics when the main step fails.
func TestRunCases_CleanupRunsOnFailure(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	handler := newMockHandler(t, ctrl)
	statusCodec := newMockStatusCodec(t, ctrl)
	factory := NewMockHarnessFactory[any](ctrl)
	factory.EXPECT().New(gomock.Any()).Times(1).Return(nil)
	inspector := NewMockErrorInspector[testStatus](ctrl)
	inspector.EXPECT().FromError(gomock.Any()).Times(1).Return(testStatusFailed, "boom message", true)

	callOrder := make([]string, 0, 4)
	handler.EXPECT().Invoke(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
		func(_ context.Context, _ any, request any) (any, error) {
			label, ok := request.(string)
			if !ok {
				return nil, errors.New("request is not a string")
			}

			callOrder = append(callOrder, label)
			if label == "action-fail" {
				return nil, errors.New("boom message")
			}

			return label, nil
		},
	)

	testCase := Case[any, testStatus]{
		Name:       "case",
		SourcePath: "cases/case.jsonc",
		Steps: []Step[any]{
			newRunnerStep("prepare-1", StepKindPrepare, "Handler", handler, "prepare", nil),
			newRunnerStep("action-1", StepKindAction, "Handler", handler, "action-fail", nil),
			newRunnerStep("verify-skip", StepKindVerify, "Handler", handler, "verify-skip", nil),
			newRunnerStep("cleanup-1", StepKindCleanup, "Handler", handler, "cleanup-1", nil),
			newRunnerStep("cleanup-2", StepKindCleanup, "Handler", handler, "cleanup-2", nil),
		},
		Assert: newRunnerAssert(testStatusFailed, "boom", nil, ""),
	}

	err := executeCases(
		t,
		[]Case[any, testStatus]{testCase},
		factory,
		inspector,
		statusCodec,
		defaultRunCasesConfig(),
	)
	require.NoError(t, err)
	require.Equal(t, []string{"prepare", "action-fail", "cleanup-1", "cleanup-2"}, callOrder)
}

// TestRunCases_NormalizeSuccess checks the normalization of a successful response for the selected target step.
func TestRunCases_NormalizeSuccess(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	handler := newMockHandler(t, ctrl)
	statusCodec := newMockStatusCodec(t, ctrl)
	factory := NewMockHarnessFactory[any](ctrl)
	factory.EXPECT().New(gomock.Any()).Times(1).Return(nil)
	inspector := NewMockErrorInspector[testStatus](ctrl)
	handler.EXPECT().Invoke(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
		func(_ context.Context, _ any, _ any) (any, error) {
			return []int{2, 1}, nil
		},
	)
	handler.EXPECT().NormalizeResponse(gomock.Any()).AnyTimes().DoAndReturn(
		func(response any) (any, error) {
			valuesRaw, ok := response.([]int)
			if !ok {
				return nil, errors.New("response is not []int")
			}
			values := append([]int(nil), valuesRaw...)
			sort.Ints(values)
			return values, nil
		},
	)

	testCase := Case[any, testStatus]{
		Name:       "case",
		SourcePath: "cases/case.jsonc",
		Steps: []Step[any]{
			newRunnerStep("action-1", StepKindAction, "Handler", handler, "act", nil),
		},
		Assert: newRunnerAssert(testStatusOK, "", []int{1, 2}, ""),
	}

	err := executeCases(
		t,
		[]Case[any, testStatus]{testCase},
		factory,
		inspector,
		statusCodec,
		defaultRunCasesConfig(),
	)
	require.NoError(t, err)
}

// TestRunCases_ResponseMismatchDefaultOmitsFullResponse checks that without env switch
// The discrepancy diagnostic remains compact and does not print the full actual answer.
func TestRunCases_ResponseMismatchDefaultOmitsFullResponse(t *testing.T) {
	t.Setenv("ITESTKIT_RESPONSE_DUMP", "")

	actual := map[string]any{
		"status":    "actual",
		"unchanged": "keep me",
	}
	expected := map[string]any{
		"status":    "expected",
		"unchanged": "keep me",
	}

	err := runResponseMismatchCase(t, actual, expected, applyRunCasesOptions(nil))
	require.Error(t, err)
	require.Contains(t, err.Error(), "response mismatch (-want +got)")
	require.False(t, json.Valid([]byte(err.Error())))
}

// TestRunCases_PartialResponseObjectSubset checks that the mode is `partial`
// allows extra object fields and does not normalize the expected response through the handler.
func TestRunCases_PartialResponseObjectSubset(t *testing.T) {
	t.Parallel()

	actual := map[string]any{
		"status": "SERVING",
		"details": map[string]any{
			"version": "v1",
			"extra":   "allowed",
		},
		"extra": "allowed",
	}
	expected := map[string]any{
		"status": "SERVING",
		"details": map[string]any{
			"version": "v1",
		},
	}

	err := runPartialResponseCase(t, actual, expected)
	require.NoError(t, err)
}

// TestRunCases_PartialResponseReportsMissingField checks for an error along the field path,
// which is in the expected response and not in the actual response.
func TestRunCases_PartialResponseReportsMissingField(t *testing.T) {
	t.Parallel()

	actual := map[string]any{
		"status": "SERVING",
	}
	expected := map[string]any{
		"status": "SERVING",
		"details": map[string]any{
			"version": "v1",
		},
	}

	err := runPartialResponseCase(t, actual, expected)
	require.Error(t, err)
	require.Contains(t, err.Error(), "response partial mismatch")
	require.Contains(t, err.Error(), "$.details")
	require.Contains(t, err.Error(), "missing field")
}

// TestRunCases_PartialResponseAbsentMarkerAllowsMissingField checks that the absence marker passes
// when the actual object does not contain the marked field.
func TestRunCases_PartialResponseAbsentMarkerAllowsMissingField(t *testing.T) {
	t.Parallel()

	actual := map[string]any{
		"status": "SERVING",
	}
	expected := map[string]any{
		"status": "SERVING",
		"debug":  map[string]any{"$absent": true},
	}

	err := runPartialResponseCase(t, actual, expected)
	require.NoError(t, err)
}

// TestRunCases_PartialResponseAbsentMarkerRejectsPresentField checks that the absence marker fails
// when the actual object contains the marked field.
func TestRunCases_PartialResponseAbsentMarkerRejectsPresentField(t *testing.T) {
	t.Parallel()

	actual := map[string]any{
		"status": "SERVING",
		"debug":  "trace",
	}
	expected := map[string]any{
		"status": "SERVING",
		"debug":  map[string]any{"$absent": true},
	}

	err := runPartialResponseCase(t, actual, expected)
	require.Error(t, err)
	require.Contains(t, err.Error(), "$.debug")
	require.Contains(t, err.Error(), "field must be absent")
}

// TestRunCases_PartialResponseAbsentMarkerRejectsNullField checks that null is treated as a present value,
// not as an absent field.
func TestRunCases_PartialResponseAbsentMarkerRejectsNullField(t *testing.T) {
	t.Parallel()

	actual := map[string]any{
		"status": "SERVING",
		"debug":  nil,
	}
	expected := map[string]any{
		"status": "SERVING",
		"debug":  map[string]any{"$absent": true},
	}

	err := runPartialResponseCase(t, actual, expected)
	require.Error(t, err)
	require.Contains(t, err.Error(), "$.debug")
	require.Contains(t, err.Error(), "field must be absent")
}

// TestRunCases_PartialResponseAbsentMarkerWorksInNestedArrayObject checks that the absence marker can be
// used inside objects that are compared through the existing strict array matching.
func TestRunCases_PartialResponseAbsentMarkerWorksInNestedArrayObject(t *testing.T) {
	t.Parallel()

	actual := map[string]any{
		"items": []any{
			map[string]any{
				"id": "1",
			},
		},
	}
	expected := map[string]any{
		"items": []any{
			map[string]any{
				"id":     "1",
				"secret": map[string]any{"$absent": true},
			},
		},
	}

	err := runPartialResponseCase(t, actual, expected)
	require.NoError(t, err)
}

// TestRunCases_PartialResponseAbsentMarkerRequiresObjectField checks that the absence marker is rejected
// when it is not used as an object field value.
func TestRunCases_PartialResponseAbsentMarkerRequiresObjectField(t *testing.T) {
	t.Parallel()

	err := runPartialResponseCase(t, "present value", map[string]any{"$absent": true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "$: absence marker can be used only as an object field value")

	err = runPartialResponseCase(t, []any{"present value"}, []any{map[string]any{"$absent": true}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "$[0]: absence marker can be used only as an object field value")
}

// TestRunCases_PartialResponsePresentMarkerAllowsAnyFieldValue checks that the presence marker ignores the present value type.
func TestRunCases_PartialResponsePresentMarkerAllowsAnyFieldValue(t *testing.T) {
	t.Parallel()

	actual := map[string]any{
		"id":      "c5033d68-8142-42e6-89a7-410066d113cd",
		"version": json.Number("12"),
		"active":  true,
		"meta": map[string]any{
			"source": "api",
		},
		"deleted_at": nil,
	}
	expected := map[string]any{
		"id":         map[string]any{"$present": true},
		"version":    map[string]any{"$present": true},
		"active":     map[string]any{"$present": true},
		"meta":       map[string]any{"$present": true},
		"deleted_at": map[string]any{"$present": true},
	}

	err := runPartialResponseCase(t, actual, expected)
	require.NoError(t, err)
}

// TestRunCases_PartialResponsePresentMarkerRejectsMissingField checks that the presence marker requires object key existence.
func TestRunCases_PartialResponsePresentMarkerRejectsMissingField(t *testing.T) {
	t.Parallel()

	err := runPartialResponseCase(t, map[string]any{"status": "SERVING"}, map[string]any{
		"status": "SERVING",
		"debug":  map[string]any{"$present": true},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "$.debug: field must be present")
}

// TestRunCases_PartialResponsePresentMarkerRequiresObjectField checks that the presence marker is not valid at root or array positions.
func TestRunCases_PartialResponsePresentMarkerRequiresObjectField(t *testing.T) {
	t.Parallel()

	err := runPartialResponseCase(t, "present value", map[string]any{"$present": true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "$: presence marker can be used only as an object field value")

	err = runPartialResponseCase(t, []any{"present value"}, []any{map[string]any{"$present": true}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "$[0]: presence marker can be used only as an object field value")
}

// TestRunCases_ExactResponsePresentMarkerAllowsAnyFieldValue checks exact-mode presence for values that cannot decode as marker strings.
func TestRunCases_ExactResponsePresentMarkerAllowsAnyFieldValue(t *testing.T) {
	t.Parallel()

	actual := map[string]any{
		"id":      "c5033d68-8142-42e6-89a7-410066d113cd",
		"version": 12.0,
		"active":  true,
		"meta": map[string]any{
			"source": "api",
		},
		"deleted_at": nil,
		"status":     "created",
	}
	expected := map[string]any{
		"id":         map[string]any{"$present": true},
		"version":    map[string]any{"$present": true},
		"active":     map[string]any{"$present": true},
		"meta":       map[string]any{"$present": true},
		"deleted_at": map[string]any{"$present": true},
		"status":     "created",
	}

	err := runExactPresentMarkerCase(t, actual, expected)
	require.NoError(t, err)
}

// TestRunCases_PartialResponsePresentMarkerAllowsStructResponseFields checks marker matching against struct JSON fields.
func TestRunCases_PartialResponsePresentMarkerAllowsStructResponseFields(t *testing.T) {
	t.Parallel()

	actual := &runnerPresentMarkerStructResponse{
		OrderID: "order-present-partial-struct",
		Amount:  4200,
		Stored:  true,
	}
	expected := map[string]any{
		"order_id": "order-present-partial-struct",
		"amount":   map[string]any{"$present": true},
		"stored":   map[string]any{"$present": true},
	}

	err := runPartialResponseCase(t, actual, expected)
	require.NoError(t, err)
}

// TestRunCases_ExactResponsePresentMarkerAllowsStructResponseFields checks marker matching against struct JSON fields.
func TestRunCases_ExactResponsePresentMarkerAllowsStructResponseFields(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	handler := NewMockHandler[any](ctrl)
	statusCodec := newMockStatusCodec(t, ctrl)
	factory := NewMockHarnessFactory[any](ctrl)
	factory.EXPECT().New(gomock.Any()).Times(1).Return(nil)
	inspector := NewMockErrorInspector[testStatus](ctrl)

	actual := &runnerPresentMarkerStructResponse{
		OrderID: "order-present-exact-struct",
		Amount:  4200,
		Stored:  true,
	}
	handler.EXPECT().Invoke(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return("raw-actual-response", nil)
	handler.EXPECT().DecodeExpectedResponse(gomock.Any()).Times(1).DoAndReturn(
		func(raw json.RawMessage) (any, error) {
			var response runnerPresentMarkerStructResponse
			if err := json.Unmarshal(raw, &response); err != nil {
				return nil, err
			}
			return &response, nil
		},
	)
	handler.EXPECT().NormalizeResponse(gomock.Any()).AnyTimes().DoAndReturn(
		func(response any) (any, error) {
			rawResponse, ok := response.(string)
			if ok && rawResponse == "raw-actual-response" {
				return actual, nil
			}
			return response, nil
		},
	)

	testCase := Case[any, testStatus]{
		Name:       "exact-present-marker-struct-case",
		SourcePath: "cases/exact-present-marker-struct-case.jsonc",
		Steps: []Step[any]{
			newRunnerStep("action-1", StepKindAction, "Handler", handler, "act", nil),
		},
		Assert: newRunnerAssert(testStatusOK, "", map[string]any{
			"order_id": "order-present-exact-struct",
			"amount":   map[string]any{"$present": true},
			"stored":   map[string]any{"$present": true},
		}, ""),
	}

	err := runCase(t.Context(), t, testCase, factory, inspector, statusCodec, defaultRunCasesConfig())
	require.NoError(t, err)
}

// TestRunCases_ExactResponsePresentMarkerRejectsMissingField checks exact-mode marker errors before expected response decoding.
func TestRunCases_ExactResponsePresentMarkerRejectsMissingField(t *testing.T) {
	t.Parallel()

	err := runExactPresentMarkerCase(t, map[string]any{"status": "created"}, map[string]any{
		"id":     map[string]any{"$present": true},
		"status": "created",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "$.id: field must be present")
}

// TestRunCases_ExactResponsePresentMarkerRejectsExtraField checks that exact mode keeps full response comparison with markers.
func TestRunCases_ExactResponsePresentMarkerRejectsExtraField(t *testing.T) {
	t.Parallel()

	err := runExactPresentMarkerCase(t, map[string]any{
		"id":     "c5033d68-8142-42e6-89a7-410066d113cd",
		"status": "created",
		"extra":  true,
	}, map[string]any{
		"id":     map[string]any{"$present": true},
		"status": "created",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "response mismatch")
}

// TestRunCases_ExactResponsePresentMarkerRequiresObjectField checks exact-mode marker placement rules.
func TestRunCases_ExactResponsePresentMarkerRequiresObjectField(t *testing.T) {
	t.Parallel()

	err := runExactPresentMarkerCase(t, "present value", map[string]any{"$present": true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "$: presence marker can be used only as an object field value")

	err = runExactPresentMarkerCase(t, []any{"present value"}, []any{map[string]any{"$present": true}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "$[0]: presence marker can be used only as an object field value")
}

// TestRunCases_PartialResponseSameInstantMatcherAcceptsEquivalentRFC3339Values checks timestamp comparison by instant.
func TestRunCases_PartialResponseSameInstantMatcherAcceptsEquivalentRFC3339Values(t *testing.T) {
	t.Parallel()

	actual := map[string]any{
		"created_at": "2026-05-30T13:00:00+03:00",
	}
	expected := map[string]any{
		"created_at": map[string]any{"$same_instant": "2026-05-30T10:00:00Z"},
	}

	err := runPartialResponseCase(t, actual, expected)
	require.NoError(t, err)
}

// TestRunCases_PartialResponseSameInstantMatcherRejectsDifferentInstant checks timestamp mismatches by instant.
func TestRunCases_PartialResponseSameInstantMatcherRejectsDifferentInstant(t *testing.T) {
	t.Parallel()

	actual := map[string]any{
		"created_at": "2026-05-30T13:00:01+03:00",
	}
	expected := map[string]any{
		"created_at": map[string]any{"$same_instant": "2026-05-30T10:00:00Z"},
	}

	err := runPartialResponseCase(t, actual, expected)
	require.Error(t, err)
	require.Contains(t, err.Error(), "$.created_at")
	require.Contains(t, err.Error(), "$same_instant")
}

// TestRunCases_PartialResponseMatchesMatcherAcceptsRegex checks string comparison by regular expression.
func TestRunCases_PartialResponseMatchesMatcherAcceptsRegex(t *testing.T) {
	t.Parallel()

	actual := map[string]any{
		"trace_id": "trace-12345",
	}
	expected := map[string]any{
		"trace_id": map[string]any{"$matches": `^trace-\d+$`},
	}

	err := runPartialResponseCase(t, actual, expected)
	require.NoError(t, err)
}

// TestRunCases_PartialResponseMatchesMatcherRejectsRegexMismatch checks regex mismatch reporting.
func TestRunCases_PartialResponseMatchesMatcherRejectsRegexMismatch(t *testing.T) {
	t.Parallel()

	actual := map[string]any{
		"trace_id": "span-12345",
	}
	expected := map[string]any{
		"trace_id": map[string]any{"$matches": `^trace-\d+$`},
	}

	err := runPartialResponseCase(t, actual, expected)
	require.Error(t, err)
	require.Contains(t, err.Error(), "$.trace_id")
	require.Contains(t, err.Error(), "$matches")
}

// TestRunCases_ExactResponseSameInstantMatcherKeepsStrictResponseComparison checks matcher materialization in exact mode.
func TestRunCases_ExactResponseSameInstantMatcherKeepsStrictResponseComparison(t *testing.T) {
	t.Parallel()

	actual := map[string]any{
		"created_at": "2026-05-30T13:00:00+03:00",
		"status":     "created",
	}
	expected := map[string]any{
		"created_at": map[string]any{"$same_instant": "2026-05-30T10:00:00Z"},
		"status":     "created",
	}

	err := runExactPresentMarkerCase(t, actual, expected)
	require.NoError(t, err)

	err = runExactPresentMarkerCase(t, map[string]any{
		"created_at": "2026-05-30T13:00:00+03:00",
		"status":     "created",
		"extra":      true,
	}, expected)
	require.Error(t, err)
	require.Contains(t, err.Error(), "response mismatch")
}

// TestRunCases_ExactResponseMatchesMatcherAcceptsRegex checks regex matcher materialization in exact mode.
func TestRunCases_ExactResponseMatchesMatcherAcceptsRegex(t *testing.T) {
	t.Parallel()

	actual := map[string]any{
		"trace_id": "trace-12345",
		"status":   "created",
	}
	expected := map[string]any{
		"trace_id": map[string]any{"$matches": `^trace-\d+$`},
		"status":   "created",
	}

	err := runExactPresentMarkerCase(t, actual, expected)
	require.NoError(t, err)
}

// TestRunCases_PartialResponseKeepsStrictArrayLength checks that the mode is `partial`
// does not turn arrays into subsets and does not skip extra elements.
func TestRunCases_PartialResponseKeepsStrictArrayLength(t *testing.T) {
	t.Parallel()

	actual := map[string]any{
		"offers": []any{
			map[string]any{"id": "1"},
			map[string]any{"id": "2"},
		},
	}
	expected := map[string]any{
		"offers": []any{
			map[string]any{"id": "1"},
		},
	}

	err := runPartialResponseCase(t, actual, expected)
	require.Error(t, err)
	require.Contains(t, err.Error(), "$.offers")
	require.Contains(t, err.Error(), "array length mismatch")
}

// TestRunCases_PartialResponseKeepsStrictArrayOrder checks that the order of the array
// remains part of the `partial` mode contract.
func TestRunCases_PartialResponseKeepsStrictArrayOrder(t *testing.T) {
	t.Parallel()

	actual := map[string]any{
		"dailies": []any{
			map[string]any{"date": "2026-03-02"},
			map[string]any{"date": "2026-03-01"},
		},
	}
	expected := map[string]any{
		"dailies": []any{
			map[string]any{"date": "2026-03-01"},
			map[string]any{"date": "2026-03-02"},
		},
	}

	err := runPartialResponseCase(t, actual, expected)
	require.Error(t, err)
	require.Contains(t, err.Error(), "$.dailies[0].date")
}

// TestRunCases_PartialResponseDistinguishesNullAndMissing checks that null
// is not equal to the missing field.
func TestRunCases_PartialResponseDistinguishesNullAndMissing(t *testing.T) {
	t.Parallel()

	actual := map[string]any{
		"status": "SERVING",
	}
	expected := map[string]any{
		"status":  "SERVING",
		"booking": nil,
	}

	err := runPartialResponseCase(t, actual, expected)
	require.Error(t, err)
	require.Contains(t, err.Error(), "$.booking")
	require.Contains(t, err.Error(), "missing field")
}

// TestRunCases_PartialResponseKeepsStrictScalarTypes checks that the mode is `partial`
// does not convert numeric strings and JSON numbers to the same value.
func TestRunCases_PartialResponseKeepsStrictScalarTypes(t *testing.T) {
	t.Parallel()

	actual := map[string]any{
		"price": "1",
	}
	expected := map[string]any{
		"price": float64(1),
	}

	err := runPartialResponseCase(t, actual, expected)
	require.Error(t, err)
	require.Contains(t, err.Error(), "$.price")
	require.Contains(t, err.Error(), "value mismatch")
}

// TestRunCases_PartialResponseMatchesTopLevelNull checks that the mode is `partial`
// allows top-level `null` as the expected JSON response.
func TestRunCases_PartialResponseMatchesTopLevelNull(t *testing.T) {
	t.Parallel()

	err := runPartialResponseCase(t, nil, nil)
	require.NoError(t, err)
}

// TestRunCases_PartialResponseKeepsJSONNumberLexeme tests strict comparison
// a numeric JSON value without casting different entries for the number.
func TestRunCases_PartialResponseKeepsJSONNumberLexeme(t *testing.T) {
	t.Parallel()

	actual := map[string]any{
		"price": json.Number("1"),
	}
	expected := map[string]any{
		"price": json.Number("1.0"),
	}

	err := runPartialResponseCase(t, actual, expected)
	require.Error(t, err)
	require.Contains(t, err.Error(), "$.price")
	require.Contains(t, err.Error(), "value mismatch")
}

// TestRunCases_PartialResponseMatchesSameJSONNumber checks for a match
// identical numeric JSON values ​​in `partial` mode.
func TestRunCases_PartialResponseMatchesSameJSONNumber(t *testing.T) {
	t.Parallel()

	actual := map[string]any{
		"price": json.Number("1"),
	}
	expected := map[string]any{
		"price": json.Number("1"),
	}

	err := runPartialResponseCase(t, actual, expected)
	require.NoError(t, err)
}

// TestRunCases_PartialResponseChecksMessageContains checks that the mode is `partial`
// keeps the `message_contains` check for a successful response.
func TestRunCases_PartialResponseChecksMessageContains(t *testing.T) {
	t.Parallel()

	actual := map[string]any{
		"status": "SERVING",
	}
	expected := map[string]any{
		"status": "SERVING",
	}

	err := runPartialResponseCaseWithAssert(t, actual, Assert[testStatus]{
		Code:             testStatusOK,
		MessageContains:  "missing-text",
		Response:         expected,
		ResponseFromStep: "",
		ResponseMode:     ResponseModePartial,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected successful response to contain")
	require.Contains(t, err.Error(), "missing-text")
}

// TestRunCases_InvalidResponseModeFromConstructedCase tests runner protection
// for a programmatically generated case with an unknown response mode.
func TestRunCases_InvalidResponseModeFromConstructedCase(t *testing.T) {
	t.Parallel()

	actual := map[string]any{
		"status": "SERVING",
	}
	expected := map[string]any{
		"status": "SERVING",
	}

	err := runPartialResponseCaseWithAssert(t, actual, Assert[testStatus]{
		Code:             testStatusOK,
		MessageContains:  "",
		Response:         expected,
		ResponseFromStep: "",
		ResponseMode:     ResponseMode("invalid"),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "field assert.response_mode")
	require.Contains(t, err.Error(), "invalid")
}

// TestRunCases_ProtoNormalizedResponseUsesProtoEquality checks protobuf semantics
// for the normalized responses that remained proto.Message.
func TestRunCases_ProtoNormalizedResponseUsesProtoEquality(t *testing.T) {
	t.Parallel()

	err := runResponseMismatchCase(t, wrapperspb.Bytes([]byte{}), wrapperspb.Bytes(nil), defaultRunCasesConfig())
	require.NoError(t, err)
}

// TestRunCases_ProtoNormalizedResponseMismatchReturnsDiff checks that protobuf is mismatched
// returns diffs instead of an error constructing diffs on internal protobuf fields.
func TestRunCases_ProtoNormalizedResponseMismatchReturnsDiff(t *testing.T) {
	t.Parallel()

	err := runResponseMismatchCase(t, wrapperspb.String("actual"), wrapperspb.String("expected"), defaultRunCasesConfig())
	require.Error(t, err)
	require.Contains(t, err.Error(), "response mismatch (-want +got)")
	require.Contains(t, err.Error(), "expected")
	require.Contains(t, err.Error(), "actual")
	require.NotContains(t, err.Error(), "cmp diff panic")
	require.NotContains(t, err.Error(), "protocmp.Message")
	require.NotContains(t, err.Error(), "Inverse(protocmp.Transform")
	require.NotContains(t, err.Error(), "map[string]any")
	require.NotContains(t, err.Error(), "[]any")
	require.NotContains(t, err.Error(), "string(")
}

// TestRunCases_NestedProtoNormalizedResponseUsesProtoEquality checks protobuf semantics
// for proto.Message inside a normalized composite value.
func TestRunCases_NestedProtoNormalizedResponseUsesProtoEquality(t *testing.T) {
	t.Parallel()

	actual := map[string]any{
		"response": wrapperspb.Bytes([]byte{}),
	}
	expected := map[string]any{
		"response": wrapperspb.Bytes(nil),
	}

	err := runResponseMismatchCase(t, actual, expected, defaultRunCasesConfig())
	require.NoError(t, err)
}

// TestRunCases_StructProtoFieldUsesProtoEquality checks that the exported protobuf field
// in the structure it is compared using protobuf semantics without false mismatches in non-exported fields.
func TestRunCases_StructProtoFieldUsesProtoEquality(t *testing.T) {
	t.Parallel()

	actual := runnerProtoStructValue{
		Response: wrapperspb.Bytes([]byte{}),
		hidden:   "same",
	}
	expected := runnerProtoStructValue{
		Response: wrapperspb.Bytes(nil),
		hidden:   "same",
	}

	err := runResponseMismatchCase(t, actual, expected, defaultRunCasesConfig())
	require.NoError(t, err)
}

// TestRunCases_StructProtoFieldIgnoresUnexportedState checks that unexported fields
// structures are not included in the JSON-like comparison of the response with protobuf.
func TestRunCases_StructProtoFieldIgnoresUnexportedState(t *testing.T) {
	t.Parallel()

	actual := runnerProtoStructValue{
		Response: wrapperspb.Bytes([]byte{}),
		hidden:   "actual",
	}
	expected := runnerProtoStructValue{
		Response: wrapperspb.Bytes(nil),
		hidden:   "expected",
	}

	err := runResponseMismatchCase(t, actual, expected, defaultRunCasesConfig())
	require.NoError(t, err)
}

// TestRunCases_NonProtoNormalizedResponseKeepsDeepEqual checks that responses are without protobuf
// Don't start using cmp.Equal to solve equality.
func TestRunCases_NonProtoNormalizedResponseKeepsDeepEqual(t *testing.T) {
	t.Parallel()

	err := runResponseMismatchCase(
		t,
		runnerEqualMethodValue{Value: "actual"},
		runnerEqualMethodValue{Value: "expected"},
		defaultRunCasesConfig(),
	)
	require.Error(t, err)
}

// TestRunCases_ResponseDumpEnvReturnsJSONBetweenMarkers checks JSON dump output
// between markers for the selected case when there is a discrepancy with the fixture.
func TestRunCases_ResponseDumpEnvReturnsJSONBetweenMarkers(t *testing.T) {
	t.Setenv("ITESTKIT_RESPONSE_DUMP", "response-mismatch-case")

	actual := map[string]any{
		"status":    "actual",
		"unchanged": "keep me",
		"details": map[string]any{
			"normalized_only": "visible",
		},
	}
	expected := map[string]any{
		"status":    "expected",
		"unchanged": "keep me",
	}

	err := runResponseMismatchCase(t, actual, expected, applyRunCasesOptions(nil))
	require.Error(t, err)

	errText := err.Error()
	dumpJSON := extractResponseDumpJSON(t, errText)
	require.True(t, json.Valid([]byte(dumpJSON)))
	require.Contains(t, dumpJSON, `"details": {`)
	require.Contains(t, dumpJSON, `"normalized_only": "visible"`)
	require.Contains(t, dumpJSON, `"unchanged": "keep me"`)
	require.NotContains(t, errText, "response mismatch")
	require.NotContains(t, errText, "raw-actual-response")
}

// TestRunCases_ResponseDumpEnvReturnsJSONForMatchingResponse checks that dump mode
// outputs the actual answer even if there is a complete match with the fixture.
func TestRunCases_ResponseDumpEnvReturnsJSONForMatchingResponse(t *testing.T) {
	t.Setenv("ITESTKIT_RESPONSE_DUMP", "response-mismatch-case")

	actual := map[string]any{
		"status":    "ok",
		"unchanged": "keep me",
	}
	expected := map[string]any{
		"status":    "ok",
		"unchanged": "keep me",
	}

	err := runResponseMismatchCase(t, actual, expected, applyRunCasesOptions(nil))
	require.Error(t, err)

	errText := err.Error()
	dumpJSON := extractResponseDumpJSON(t, errText)
	require.True(t, json.Valid([]byte(dumpJSON)))
	require.Contains(t, dumpJSON, `"status": "ok"`)
	require.Contains(t, dumpJSON, `"unchanged": "keep me"`)
	require.NotContains(t, errText, "response mismatch")
}

// TestRunCases_ResponseDumpEnvReturnsProtoAsJSON checks that dump mode
// outputs proto.Message as a JSON representation of the response without internal protobuf fields.
func TestRunCases_ResponseDumpEnvReturnsProtoAsJSON(t *testing.T) {
	t.Setenv("ITESTKIT_RESPONSE_DUMP", "response-mismatch-case")

	err := runResponseMismatchCase(t, wrapperspb.String("actual"), wrapperspb.String("expected"), applyRunCasesOptions(nil))
	require.Error(t, err)

	dumpJSON := extractResponseDumpJSON(t, err.Error())
	require.JSONEq(t, `"actual"`, dumpJSON)
	require.NotContains(t, dumpJSON, "state")
	require.NotContains(t, dumpJSON, "sizeCache")
	require.NotContains(t, dumpJSON, "unknownFields")
}

// TestRunCases_ResponseDumpEnvSkipsExpectedResponseNormalization checks that dump mode
// does not depend on assert.response and removes the actual response without comparing it to the fixture.
func TestRunCases_ResponseDumpEnvSkipsExpectedResponseNormalization(t *testing.T) {
	t.Setenv("ITESTKIT_RESPONSE_DUMP", "response-dump-case")

	ctrl := gomock.NewController(t)
	handler := newMockHandler(t, ctrl)
	statusCodec := newMockStatusCodec(t, ctrl)
	factory := NewMockHarnessFactory[any](ctrl)
	factory.EXPECT().New(gomock.Any()).Times(1).Return(nil)
	inspector := NewMockErrorInspector[testStatus](ctrl)

	handler.EXPECT().Invoke(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return("raw-actual-response", nil)
	handler.EXPECT().NormalizeResponse("raw-actual-response").Times(1).Return(map[string]any{
		"status": "actual",
	}, nil)
	handler.EXPECT().NormalizeResponse("broken-fixture-response").Times(0)

	testCase := Case[any, testStatus]{
		Name:       "response-dump-case",
		SourcePath: "cases/response-dump-case.jsonc",
		Steps: []Step[any]{
			newRunnerStep("action-1", StepKindAction, "Handler", handler, "act", nil),
		},
		Assert: newRunnerAssert(testStatusOK, "", "broken-fixture-response", ""),
	}

	err := runCase(t.Context(), t, testCase, factory, inspector, statusCodec, applyRunCasesOptions(nil))
	require.Error(t, err)

	dumpJSON := extractResponseDumpJSON(t, err.Error())
	require.JSONEq(t, `{"status":"actual"}`, dumpJSON)
}

// TestRunCases_ResponseMismatchEnvReportsJSONMarshalError checks that the error
// Serialization of diagnostic JSON is returned from dump mode.
func TestRunCases_ResponseMismatchEnvReportsJSONMarshalError(t *testing.T) {
	t.Setenv("ITESTKIT_RESPONSE_DUMP", "response-mismatch-case")

	actual := map[string]any{
		"price": math.Inf(1),
	}
	expected := map[string]any{
		"price": float64(100),
	}

	err := runResponseMismatchCase(t, actual, expected, applyRunCasesOptions(nil))
	require.Error(t, err)

	errText := err.Error()
	require.Contains(t, errText, "response dump failed:")
	require.Contains(t, errText, "unsupported value: +Inf")
	require.NotContains(t, errText, "response mismatch")
}

// TestRunCases_DynamicRequestTemplateSuccess checks the request substitution from the output of the previous step.
func TestRunCases_DynamicRequestTemplateSuccess(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	sourceHandler := NewMockHandler[any](ctrl)
	dynamicHandler := NewMockHandler[any](ctrl)
	statusCodec := newMockStatusCodec(t, ctrl)
	factory := NewMockHarnessFactory[any](ctrl)
	factory.EXPECT().New(gomock.Any()).Times(1).Return(nil)
	inspector := NewMockErrorInspector[testStatus](ctrl)

	sourceHandler.EXPECT().Invoke(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).DoAndReturn(
		func(_ context.Context, _ any, request any) (any, error) {
			if request != "action-request" {
				return nil, errors.New("unexpected source request")
			}
			return map[string]any{
				"order_id": "order-100",
				"amount":   4200,
			}, nil
		},
	)
	dynamicHandler.EXPECT().DecodeRequest(gomock.Any()).Times(1).DoAndReturn(
		func(raw json.RawMessage) (any, error) {
			return decodeJSON(raw)
		},
	)
	dynamicHandler.EXPECT().Invoke(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).DoAndReturn(
		func(_ context.Context, _ any, request any) (any, error) {
			requestMap, ok := request.(map[string]any)
			if !ok {
				return nil, errors.New("resolved request is not a map")
			}
			if requestMap["order_id"] != "order-100" {
				return nil, errors.New("unexpected order_id after template resolution")
			}
			if requestMap["amount"] != float64(4200) {
				return nil, errors.New("unexpected amount after template resolution")
			}
			return "verified", nil
		},
	)
	dynamicHandler.EXPECT().NormalizeResponse(gomock.Any()).AnyTimes().DoAndReturn(
		func(response any) (any, error) {
			return response, nil
		},
	)

	testCase := Case[any, testStatus]{
		Name:       "dynamic-request-case",
		SourcePath: "cases/dynamic-request-case.jsonc",
		Steps: []Step[any]{
			newRunnerStep("action-1", StepKindAction, "Source", sourceHandler, "action-request", nil),
			newRunnerStep(
				"verify-1",
				StepKindVerify,
				"Dynamic",
				dynamicHandler,
				stepDynamicRequest{
					raw: json.RawMessage(`{
						"order_id":"{{steps.action-1.response.order_id}}",
						"amount":"{{steps.action-1.response.amount}}"
					}`),
				},
				nil,
			),
		},
		Assert: newRunnerAssert(testStatusOK, "", "verified", "verify-1"),
	}

	err := executeCases(
		t,
		[]Case[any, testStatus]{testCase},
		factory,
		inspector,
		statusCodec,
		defaultRunCasesConfig(),
	)
	require.NoError(t, err)
}

// TestRunCases_DynamicRequestTemplateMissingOutput checks diagnostics when referencing a step's unavailable output.
func TestRunCases_DynamicRequestTemplateMissingOutput(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	handler := NewMockHandler[any](ctrl)
	statusCodec := newMockStatusCodec(t, ctrl)
	factory := NewMockHarnessFactory[any](ctrl)
	factory.EXPECT().New(gomock.Any()).Times(1).Return(nil)

	handler.EXPECT().DecodeRequest(gomock.Any()).Times(0)
	handler.EXPECT().Invoke(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	testCase := Case[any, testStatus]{
		Name:       "dynamic-request-missing-output",
		SourcePath: "cases/dynamic-request-missing-output.jsonc",
		Steps: []Step[any]{
			newRunnerStep(
				"verify-1",
				StepKindVerify,
				"Dynamic",
				handler,
				stepDynamicRequest{raw: json.RawMessage(`{"order_id":"{{steps.missing-step.response.order_id}}"}`)},
				nil,
			),
		},
		Assert: newRunnerAssert(testStatusOK, "", "unused", "verify-1"),
	}

	err := runCase(t.Context(), t, testCase, factory, nil, statusCodec, defaultRunCasesConfig())
	require.Error(t, err)
	require.Contains(t, err.Error(), "resolve request")
	require.Contains(t, err.Error(), "request template references step \"missing-step\"")
}

// TestWithSuiteLifecycle_TeardownOnPanic checks for teardown even when panicking inside runner-callback.
func TestWithSuiteLifecycle_TeardownOnPanic(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	lifecycle := NewMockSuiteLifecycle[string](ctrl)
	lifecycle.EXPECT().SetupSuite(gomock.Any()).Times(1).Return("suite", nil)
	lifecycle.EXPECT().TeardownSuite(gomock.Any(), "suite").Times(1).Return(nil)

	require.Panics(t, func() {
		_ = withSuiteLifecycle(t, lifecycle, func(_ string) error {
			panic("boom")
		})
	})
}

// TestRunCases_SuiteSetupTeardownOnce checks setup/teardown once semantics in suite mode.
func TestRunCases_SuiteSetupTeardownOnce(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	statusCodec := newMockStatusCodec(t, ctrl)
	inspector := NewMockErrorInspector[testStatus](ctrl)
	handler := newMockHandler(t, ctrl)

	suiteLog := make([]string, 0, 2)
	suiteContext := &suiteLog

	lifecycle := NewMockSuiteLifecycle[*[]string](ctrl)
	lifecycle.EXPECT().SetupSuite(gomock.Any()).Times(1).Return(suiteContext, nil)
	lifecycle.EXPECT().TeardownSuite(gomock.Any(), suiteContext).Times(1).Return(nil)

	caseFactory := NewMockSuiteCaseHarnessFactory[*[]string, any](ctrl)
	caseFactory.EXPECT().NewCaseHarness(gomock.Any(), suiteContext).Times(2).Return(suiteContext)

	handler.EXPECT().Invoke(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
		func(_ context.Context, currentHarness any, request any) (any, error) {
			harnessLog, ok := currentHarness.(*[]string)
			if !ok {
				return nil, errors.New("harness is not *[]string")
			}

			label, ok := request.(string)
			if !ok {
				return nil, errors.New("request is not a string")
			}

			*harnessLog = append(*harnessLog, label)
			return label, nil
		},
	)
	handler.EXPECT().NormalizeResponse(gomock.Any()).AnyTimes().DoAndReturn(
		func(response any) (any, error) {
			return response, nil
		},
	)

	testCases := []Case[any, testStatus]{
		{
			Name:       "case-1",
			SourcePath: "cases/case-1.jsonc",
			Steps: []Step[any]{
				newRunnerStep("a-1", StepKindAction, "Handler", handler, "case-1-action", nil),
			},
			Assert: newRunnerAssert(testStatusOK, "", "case-1-action", "a-1"),
		},
		{
			Name:       "case-2",
			SourcePath: "cases/case-2.jsonc",
			Steps: []Step[any]{
				newRunnerStep("a-2", StepKindAction, "Handler", handler, "case-2-action", nil),
			},
			Assert: newRunnerAssert(testStatusOK, "", "case-2-action", "a-2"),
		},
	}

	RunCases(t, testCases, lifecycle, caseFactory, inspector, statusCodec)
	require.Equal(t, []string{"case-1-action", "case-2-action"}, suiteLog)
}

// TestRunCases_SuiteTeardownRunsOnFailure checks the teardown guarantee when a case fails.
func TestRunCases_SuiteTeardownRunsOnFailure(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	statusCodec := newMockStatusCodec(t, ctrl)
	inspector := NewMockErrorInspector[testStatus](ctrl)
	handler := newMockHandler(t, ctrl)

	suiteContext := "suite"
	lifecycle := NewMockSuiteLifecycle[string](ctrl)
	lifecycle.EXPECT().SetupSuite(gomock.Any()).Times(1).Return(suiteContext, nil)
	lifecycle.EXPECT().TeardownSuite(gomock.Any(), suiteContext).Times(1).Return(nil)

	caseFactory := NewMockSuiteCaseHarnessFactory[string, any](ctrl)
	caseFactory.EXPECT().NewCaseHarness(gomock.Any(), suiteContext).Times(1).Return(nil)

	handler.EXPECT().Invoke(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
		func(_ context.Context, _ any, _ any) (any, error) {
			return nil, errors.New("boom")
		},
	)

	testCase := Case[any, testStatus]{
		Name:       "case-fail",
		SourcePath: "cases/case-fail.jsonc",
		Steps: []Step[any]{
			newRunnerStep("a-1", StepKindAction, "Handler", handler, "action", nil),
		},
		Assert: newRunnerAssert(testStatusOK, "", nil, ""),
	}

	err := withSuiteLifecycle(t, lifecycle, func(currentSuiteContext string) error {
		harnessFactory := suiteHarnessFactory[string, any]{
			suiteContext: currentSuiteContext,
			caseFactory:  caseFactory,
		}

		return runCase(t.Context(), t, testCase, harnessFactory, inspector, statusCodec, defaultRunCasesConfig())
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected error")
}

// TestExecuteCasesWithRunner_FailFastStopsAfterFirstFailure checks fail-fast mode.
func TestExecuteCasesWithRunner_FailFastStopsAfterFirstFailure(t *testing.T) {
	t.Parallel()

	calls := make([]string, 0, 3)
	cases := []Case[any, testStatus]{
		newRunnerPolicyTestCase("case-1"),
		newRunnerPolicyTestCase("case-2"),
		newRunnerPolicyTestCase("case-3"),
	}
	config := defaultRunCasesConfig()

	err := executeCasesWithRunner(context.Background(), cases, func(_ context.Context, testCase Case[any, testStatus]) error {
		calls = append(calls, testCase.Name)
		if testCase.Name == "case-1" {
			return errors.New("boom-1")
		}

		return nil
	}, config)

	require.Error(t, err)
	require.Equal(t, []string{"case-1"}, calls)
	require.Contains(t, err.Error(), "boom-1")
}

// TestExecuteCasesWithRunner_ContinueCollectsAllFailures tests the collection of all errors.
func TestExecuteCasesWithRunner_ContinueCollectsAllFailures(t *testing.T) {
	t.Parallel()

	calls := make([]string, 0, 3)
	cases := []Case[any, testStatus]{
		newRunnerPolicyTestCase("case-1"),
		newRunnerPolicyTestCase("case-2"),
		newRunnerPolicyTestCase("case-3"),
	}
	config := defaultRunCasesConfig()
	config.failFast = false

	err := executeCasesWithRunner(context.Background(), cases, func(_ context.Context, testCase Case[any, testStatus]) error {
		calls = append(calls, testCase.Name)
		switch testCase.Name {
		case "case-1":
			return errors.New("boom-1")
		case "case-3":
			return errors.New("boom-3")
		default:
			return nil
		}
	}, config)

	require.Error(t, err)
	require.Equal(t, []string{"case-1", "case-2", "case-3"}, calls)

	errText := err.Error()
	require.True(t, strings.Contains(errText, "case case-1 failed") || strings.Contains(errText, "boom-1"))
	require.True(t, strings.Contains(errText, "case case-3 failed") || strings.Contains(errText, "boom-3"))
}

// TestExecuteCases_ParallelContinueReturnsAggregatedError checks that parallel+continue
// returns the aggregated error to the main thread of execution.
//
//nolint:tparallel // Here we need a synchronous subtest: t.Parallel breaks the aggregated error check in the parent.
func TestExecuteCases_ParallelContinueReturnsAggregatedError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	factory := NewMockHarnessFactory[any](ctrl)
	factory.EXPECT().New(gomock.Any()).Times(2).Return(nil)
	inspector := NewMockErrorInspector[testStatus](ctrl)
	statusCodec := newMockStatusCodec(t, ctrl)

	cases := []Case[any, testStatus]{
		newRunnerPolicyTestCase("case-1"),
		newRunnerPolicyTestCase("case-2"),
	}
	config := defaultRunCasesConfig()
	config.parallel = true
	config.failFast = false
	config.parallelismLimit = 2

	executionErr := runParallelExecuteCasesSubtest(t, cases, factory, inspector, statusCodec, config)
	require.Error(t, executionErr)
	require.Contains(t, executionErr.Error(), "--- CASE \"case-1\" FAILED")
	require.Contains(t, executionErr.Error(), "--- CASE \"case-2\" FAILED")
}

// TestApplyRunCasesOptions_ParallelDefaults checks the default limit in parallel mode.
func TestApplyRunCasesOptions_ParallelDefaults(t *testing.T) {
	t.Parallel()

	config := applyRunCasesOptions([]RunCasesOption{WithParallelCases()})

	require.True(t, config.parallel)
	require.True(t, config.failFast)
	require.Equal(t, DefaultParallelCasesLimit, config.effectiveParallelismLimit())
}

// TestApplyRunCasesOptions_ParallelLimit tests the setting of the parallelism limit.
func TestApplyRunCasesOptions_ParallelLimit(t *testing.T) {
	t.Parallel()

	config := applyRunCasesOptions([]RunCasesOption{WithParallelismLimit(3)})

	require.True(t, config.parallel)
	require.Equal(t, 3, config.effectiveParallelismLimit())
}

// TestApplyRunCasesOptions_ParallelLimitFallback checks the fallback against the default limit.
func TestApplyRunCasesOptions_ParallelLimitFallback(t *testing.T) {
	t.Parallel()

	config := applyRunCasesOptions([]RunCasesOption{WithParallelismLimit(0)})

	require.True(t, config.parallel)
	require.Equal(t, DefaultParallelCasesLimit, config.effectiveParallelismLimit())
}

// TestApplyRunCasesOptions_ResponseDumpDisablesParallel checks that the dump mode is
// disables parallel running regardless of user options.
func TestApplyRunCasesOptions_ResponseDumpDisablesParallel(t *testing.T) {
	t.Setenv("ITESTKIT_RESPONSE_DUMP", "response-mismatch-case")

	config := applyRunCasesOptions([]RunCasesOption{WithParallelCases(), WithParallelismLimit(2)})

	require.Equal(t, "response-mismatch-case", config.responseDumpCaseName)
	require.False(t, config.parallel)
}

// TestExecuteCases_ResponseDumpCaseNotFoundSkipsPackage checks that the package without the selected
// The case does not crash in the general startup dump mode.
func TestExecuteCases_ResponseDumpCaseNotFoundSkipsPackage(t *testing.T) {
	t.Parallel()

	config := defaultRunCasesConfig()
	config.responseDumpCaseName = "missing-case"

	err := executeCases[any, testStatus](
		t,
		[]Case[any, testStatus]{newRunnerPolicyTestCase("existing-case")},
		nil,
		nil,
		nil,
		config,
	)

	require.NoError(t, err)
}

// runParallelExecuteCasesSubtest runs executeCases in a separate subtest to collect the aggregated error from t.Run.
func runParallelExecuteCasesSubtest(
	t *testing.T,
	cases []Case[any, testStatus],
	factory HarnessFactory[any],
	inspector ErrorInspector[testStatus],
	statusCodec StatusCodec[testStatus],
	config runCasesConfig,
) error {
	t.Helper()

	var executionErr error
	if ok := t.Run("parallel-run", func(t *testing.T) {
		executionErr = executeCases(t, cases, factory, inspector, statusCodec, config)
	}); !ok {
		t.FailNow()
	}

	return executionErr
}

// newRunnerPolicyTestCase creates a minimally valid case for run policy tests.
func newRunnerPolicyTestCase(name string) Case[any, testStatus] {
	return Case[any, testStatus]{
		Name:       name,
		SourcePath: "cases/" + name + ".jsonc",
		Steps: []Step[any]{
			newRunnerStep("policy-action", StepKindAction, "Policy", nil, nil, nil),
		},
		Assert: newRunnerAssert(testStatusOK, "", nil, ""),
	}
}

// runExactPresentMarkerCase runs an exact-mode case where expected response may contain presence markers.
func runExactPresentMarkerCase(t *testing.T, actualNormalized, expectedTemplate any) error {
	t.Helper()

	ctrl := gomock.NewController(t)
	handler := newMockHandler(t, ctrl)
	statusCodec := newMockStatusCodec(t, ctrl)
	factory := NewMockHarnessFactory[any](ctrl)
	factory.EXPECT().New(gomock.Any()).Times(1).Return(nil)
	inspector := NewMockErrorInspector[testStatus](ctrl)

	handler.EXPECT().Invoke(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return("raw-actual-response", nil)
	handler.EXPECT().NormalizeResponse(gomock.Any()).AnyTimes().DoAndReturn(
		func(response any) (any, error) {
			rawResponse, ok := response.(string)
			if ok && rawResponse == "raw-actual-response" {
				return actualNormalized, nil
			}
			return response, nil
		},
	)

	testCase := Case[any, testStatus]{
		Name:       "exact-present-marker-case",
		SourcePath: "cases/exact-present-marker-case.jsonc",
		Steps: []Step[any]{
			newRunnerStep("action-1", StepKindAction, "Handler", handler, "act", nil),
		},
		Assert: newRunnerAssert(testStatusOK, "", expectedTemplate, ""),
	}

	return runCase(t.Context(), t, testCase, factory, inspector, statusCodec, defaultRunCasesConfig())
}

// runResponseMismatchCase runs a minimal case with mismatched normalized responses.
func runResponseMismatchCase(t *testing.T, actualNormalized, expectedNormalized any, config runCasesConfig) error {
	t.Helper()

	ctrl := gomock.NewController(t)
	handler := newMockHandler(t, ctrl)
	statusCodec := newMockStatusCodec(t, ctrl)
	factory := NewMockHarnessFactory[any](ctrl)
	factory.EXPECT().New(gomock.Any()).Times(1).Return(nil)
	inspector := NewMockErrorInspector[testStatus](ctrl)

	handler.EXPECT().Invoke(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return("raw-actual-response", nil)
	handler.EXPECT().NormalizeResponse(gomock.Any()).AnyTimes().DoAndReturn(
		func(response any) (any, error) {
			switch response {
			case "raw-actual-response":
				return actualNormalized, nil
			case "fixture-expected-response":
				return expectedNormalized, nil
			default:
				return response, nil
			}
		},
	)

	testCase := Case[any, testStatus]{
		Name:       "response-mismatch-case",
		SourcePath: "cases/response-mismatch-case.jsonc",
		Steps: []Step[any]{
			newRunnerStep("action-1", StepKindAction, "Handler", handler, "act", nil),
		},
		Assert: newRunnerAssert(testStatusOK, "", "fixture-expected-response", ""),
	}

	return runCase(t.Context(), t, testCase, factory, inspector, statusCodec, config)
}

// runPartialResponseCase runs a minimal case with response comparison in `partial` mode.
func runPartialResponseCase(t *testing.T, actualNormalized, expectedPartial any) error {
	t.Helper()

	return runPartialResponseCaseWithAssert(t, actualNormalized, Assert[testStatus]{
		Code:             testStatusOK,
		MessageContains:  "",
		Response:         expectedPartial,
		ResponseFromStep: "",
		ResponseMode:     ResponseModePartial,
	})
}

// runPartialResponseCaseWithAssert runs a minimal case with the given assert block.
func runPartialResponseCaseWithAssert(t *testing.T, actualNormalized any, assertion Assert[testStatus]) error {
	t.Helper()

	ctrl := gomock.NewController(t)
	handler := newMockHandler(t, ctrl)
	statusCodec := newMockStatusCodec(t, ctrl)
	factory := NewMockHarnessFactory[any](ctrl)
	factory.EXPECT().New(gomock.Any()).Times(1).Return(nil)
	inspector := NewMockErrorInspector[testStatus](ctrl)

	handler.EXPECT().Invoke(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return("raw-actual-response", nil)
	handler.EXPECT().NormalizeResponse("raw-actual-response").Times(1).Return(actualNormalized, nil)
	handler.EXPECT().NormalizeResponse(assertion.Response).Times(0)

	testCase := Case[any, testStatus]{
		Name:       "partial-response-case",
		SourcePath: "cases/partial-response-case.jsonc",
		Steps: []Step[any]{
			newRunnerStep("action-1", StepKindAction, "Handler", handler, "act", nil),
		},
		Assert: assertion,
	}

	return runCase(t.Context(), t, testCase, factory, inspector, statusCodec, defaultRunCasesConfig())
}

// extractResponseDumpJSON returns JSON between dump mode markers.
func extractResponseDumpJSON(t *testing.T, output string) string {
	t.Helper()

	const beginMarker = "ITESTKIT_RESPONSE_DUMP_BEGIN"
	const endMarker = "ITESTKIT_RESPONSE_DUMP_END"

	begin := strings.Index(output, beginMarker)
	require.NotEqual(t, -1, begin)
	begin += len(beginMarker)

	end := strings.Index(output[begin:], endMarker)
	require.NotEqual(t, -1, end)

	return strings.TrimSpace(output[begin : begin+end])
}

// newRunnerStep creates a fully populated Step for runner tests.
func newRunnerStep(
	id string,
	kind StepKind,
	handlerName string,
	handler Handler[any],
	request any,
	retry *AwaitRetry,
) Step[any] {
	return Step[any]{
		ID:          id,
		Kind:        kind,
		HandlerName: handlerName,
		Handler:     handler,
		Request:     request,
		Retry:       retry,
	}
}

// newRunnerAssert creates a fully populated Assert for runner tests.
func newRunnerAssert(code testStatus, messageContains string, response any, responseFromStep string) Assert[testStatus] {
	return Assert[testStatus]{
		Code:             code,
		MessageContains:  messageContains,
		Response:         response,
		ResponseFromStep: responseFromStep,
		ResponseMode:     "",
	}
}
