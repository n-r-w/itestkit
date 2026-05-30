// Package itestkit is a portable core for running integration tests on JSONC cases.
package itestkit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/google/go-cmp/cmp"
	"github.com/pmezard/go-difflib/difflib"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
	"google.golang.org/protobuf/proto"

	"github.com/n-r-w/itestkit/internal/protojsonview"
)

// DefaultParallelCasesLimit sets the default limit of parallel cases when parallel mode is enabled.
const DefaultParallelCasesLimit = 10

// DefaultJSONDiffContextLines specifies the number of context lines in the JSON-like diff output.
const DefaultJSONDiffContextLines = 3

// responseDumpEnvVar specifies the name of the case for which a JSON dump of the actual response is needed.
const responseDumpEnvVar = "ITESTKIT_RESPONSE_DUMP"

// RunCasesOption configures the behavior of RunCases.
type RunCasesOption func(*runCasesConfig)

// suiteHarnessFactory associates a suite context with a case-level harness factory.
type suiteHarnessFactory[SC any, C any] struct {
	suiteContext SC
	caseFactory  SuiteCaseHarnessFactory[SC, C]
}

// New creates a case-level harness through a SuiteCaseHarnessFactory and a committed suite context.
func (factory suiteHarnessFactory[SC, C]) New(t *testing.T) C {
	return factory.caseFactory.NewCaseHarness(t, factory.suiteContext)
}

// runCasesConfig stores parameters for running cases.
type runCasesConfig struct {
	failFast             bool
	parallel             bool
	parallelismLimit     int
	responseDumpCaseName string
}

// defaultRunCasesConfig returns the default configuration.
func defaultRunCasesConfig() runCasesConfig {
	return runCasesConfig{
		failFast:             true,
		parallel:             false,
		parallelismLimit:     DefaultParallelCasesLimit,
		responseDumpCaseName: strings.TrimSpace(os.Getenv(responseDumpEnvVar)),
	}
}

// effectiveParallelismLimit returns the current limit of parallel cases.
func (config runCasesConfig) effectiveParallelismLimit() int {
	if config.parallelismLimit > 0 {
		return config.parallelismLimit
	}

	return DefaultParallelCasesLimit
}

// applyRunCasesOptions applies custom run options.
func applyRunCasesOptions(options []RunCasesOption) runCasesConfig {
	config := defaultRunCasesConfig()
	for _, option := range options {
		if option == nil {
			continue
		}
		option(&config)
	}
	if config.responseDumpCaseName != "" {
		config.parallel = false
	}

	return config
}

// WithContinueOnFailure disables fail-fast and runs all cases to completion.
func WithContinueOnFailure() RunCasesOption {
	return func(config *runCasesConfig) {
		config.failFast = false
	}
}

// WithParallelCases enables parallel running of cases.
// The limit of parallel cases can be configured via WithParallelismLimit.
// By default, the limit is DefaultParallelCasesLimit.
func WithParallelCases() RunCasesOption {
	return func(config *runCasesConfig) {
		config.parallel = true
	}
}

// WithParallelismLimit sets the limit of parallel cases and enables parallel mode.
func WithParallelismLimit(limit int) RunCasesOption {
	return func(config *runCasesConfig) {
		config.parallel = true
		if limit <= 0 {
			return
		}
		config.parallelismLimit = limit
	}
}

// RunCases executes a set of cases through a suite lifecycle (setup once / teardown once).
func RunCases[SC any, C any, S comparable](
	t *testing.T,
	cases []Case[C, S],
	lifecycle SuiteLifecycle[SC],
	caseFactory SuiteCaseHarnessFactory[SC, C],
	inspector ErrorInspector[S],
	statusCodec StatusCodec[S],
	options ...RunCasesOption,
) {
	config := applyRunCasesOptions(options)
	err := withSuiteLifecycle(t, lifecycle, func(suiteContext SC) error {
		harnessFactory := suiteHarnessFactory[SC, C]{
			suiteContext: suiteContext,
			caseFactory:  caseFactory,
		}

		return executeCases(t, cases, harnessFactory, inspector, statusCodec, config)
	})
	if err != nil {
		t.Fatalf("\n%s", err)
	}
}

// withSuiteLifecycle performs setup/teardown suite and guarantees teardown even in panic.
func withSuiteLifecycle[SC any](
	t *testing.T,
	lifecycle SuiteLifecycle[SC],
	run func(suiteContext SC) error,
) (err error) {
	t.Helper()

	suiteContext, err := lifecycle.SetupSuite(t)
	if err != nil {
		return fmt.Errorf("setup suite: %w", err)
	}

	defer func() {
		teardownErr := lifecycle.TeardownSuite(t, suiteContext)
		if teardownErr != nil {
			wrappedTeardownErr := fmt.Errorf("teardown suite: %w", teardownErr)
			if err == nil {
				err = wrappedTeardownErr
			} else {
				err = errors.Join(err, wrappedTeardownErr)
			}
		}

		if recovered := recover(); recovered != nil {
			panic(recovered)
		}
	}()

	return run(suiteContext)
}

// executeCases executes cases according to the fail-fast/continue settings.
func executeCases[C any, S comparable](
	t *testing.T,
	cases []Case[C, S],
	harnessFactory HarnessFactory[C],
	inspector ErrorInspector[S],
	statusCodec StatusCodec[S],
	config runCasesConfig,
) error {
	cases = selectResponseDumpCase(cases, config.responseDumpCaseName)

	if config.parallel {
		return executeCasesInParallel(t, cases, harnessFactory, inspector, statusCodec, config)
	}

	runner := func(runContext context.Context, testCase Case[C, S]) error {
		var caseErr error
		ok := t.Run(testCase.Name, func(t *testing.T) {
			if err := runCase(runContext, t, testCase, harnessFactory, inspector, statusCodec, config); err != nil {
				caseErr = err
				t.Fail()
			}
		})

		return buildCaseExecutionError(testCase.Name, ok, caseErr)
	}

	return executeCasesWithRunner(t.Context(), cases, runner, config)
}

// selectResponseDumpCase leaves only the case for which a response dump was requested.
func selectResponseDumpCase[C any, S comparable](
	cases []Case[C, S],
	caseName string,
) []Case[C, S] {
	if caseName == "" {
		return cases
	}

	for _, testCase := range cases {
		if testCase.Name == caseName {
			return []Case[C, S]{testCase}
		}
	}

	return nil
}

// executeCasesInParallel runs cases in parallel and supports cancellation on first fail.
func executeCasesInParallel[C any, S comparable](
	t *testing.T,
	cases []Case[C, S],
	harnessFactory HarnessFactory[C],
	inspector ErrorInspector[S],
	statusCodec StatusCodec[S],
	config runCasesConfig,
) error {
	if config.failFast {
		return executeCasesInParallelFailFast(t, cases, harnessFactory, inspector, statusCodec, config)
	}

	return executeCasesInParallelContinue(t, cases, harnessFactory, inspector, statusCodec, config)
}

// executeCasesInParallelFailFast runs cases in parallel and returns the first error.
func executeCasesInParallelFailFast[C any, S comparable](
	t *testing.T,
	cases []Case[C, S],
	harnessFactory HarnessFactory[C],
	inspector ErrorInspector[S],
	statusCodec StatusCodec[S],
	config runCasesConfig,
) error {
	group, runContext := errgroup.WithContext(t.Context())
	group.SetLimit(config.effectiveParallelismLimit())

	for _, caseValue := range cases {
		testCase := caseValue
		group.Go(func() error {
			ok, caseErr := runParallelCase(runContext, t, testCase, harnessFactory, inspector, statusCodec, config)
			if errors.Is(caseErr, context.Canceled) && runContext.Err() != nil {
				caseErr = nil
			}

			return buildCaseExecutionError(testCase.Name, ok, caseErr)
		})
	}

	return group.Wait()
}

// executeCasesInParallelContinue runs all cases in parallel and returns an aggregated error.
func executeCasesInParallelContinue[C any, S comparable](
	t *testing.T,
	cases []Case[C, S],
	harnessFactory HarnessFactory[C],
	inspector ErrorInspector[S],
	statusCodec StatusCodec[S],
	config runCasesConfig,
) error {
	runContext := t.Context()
	limiter := semaphore.NewWeighted(int64(config.effectiveParallelismLimit()))

	var errorsMu sync.Mutex
	executionErrors := make([]error, 0, len(cases))
	appendExecutionError := func(err error) {
		errorsMu.Lock()
		executionErrors = append(executionErrors, err)
		errorsMu.Unlock()
	}

	var waitGroup sync.WaitGroup

	for _, caseValue := range cases {
		testCase := caseValue
		waitGroup.Go(func() {
			if err := limiter.Acquire(runContext, 1); err != nil {
				return
			}
			defer func() {
				limiter.Release(1)
			}()

			ok, caseErr := runParallelCase(runContext, t, testCase, harnessFactory, inspector, statusCodec, config)
			if err := buildCaseExecutionError(testCase.Name, ok, caseErr); err != nil {
				appendExecutionError(err)
			}
		})
	}

	waitGroup.Wait()

	errorsMu.Lock()
	joinedErr := errors.Join(executionErrors...)
	errorsMu.Unlock()

	if joinedErr == nil {
		return nil
	}

	return joinedErr
}

// runParallelCase runs the case in subtest and returns a business error along with the result t.Run.
func runParallelCase[C any, S comparable](
	runContext context.Context,
	t *testing.T,
	testCase Case[C, S],
	harnessFactory HarnessFactory[C],
	inspector ErrorInspector[S],
	statusCodec StatusCodec[S],
	config runCasesConfig,
) (ok bool, caseErr error) {
	ok = t.Run(testCase.Name, func(t *testing.T) {
		caseErr = runCase(runContext, t, testCase, harnessFactory, inspector, statusCodec, config)
	})

	return ok, caseErr
}

// buildCaseExecutionError generates the final case execution error.
func buildCaseExecutionError(caseName string, ok bool, caseErr error) error {
	if caseErr != nil {
		return fmt.Errorf("--- CASE %q FAILED: ---\n%w", caseName, caseErr)
	}
	if !ok {
		return fmt.Errorf("--- CASE %q FAILED ---", caseName)
	}

	return nil
}

// executeCasesWithRunner controls fail-fast and aggregation of case execution errors.
func executeCasesWithRunner[C any, S comparable](
	runContext context.Context,
	cases []Case[C, S],
	runner func(runContext context.Context, testCase Case[C, S]) error,
	config runCasesConfig,
) error {
	executionErrors := make([]error, 0, len(cases))

	for _, testCase := range cases {
		err := runner(runContext, testCase)
		if err == nil {
			continue
		}
		if config.failFast {
			return err
		}
		executionErrors = append(executionErrors, err)
	}

	if len(executionErrors) == 0 {
		return nil
	}

	return errors.Join(executionErrors...)
}

// runCase executes one case and returns an error if expectations are not met.
func runCase[C any, S comparable](
	runContext context.Context,
	t *testing.T,
	testCase Case[C, S],
	harnessFactory HarnessFactory[C],
	inspector ErrorInspector[S],
	statusCodec StatusCodec[S],
	config runCasesConfig,
) error {
	t.Helper()

	harness := harnessFactory.New(t)

	stepOutputs := make(map[string]any, len(testCase.Steps))
	stepsByID := make(map[string]Step[C], len(testCase.Steps))
	lastActionStepID := ""
	var executionErr error

	for _, step := range testCase.Steps {
		stepsByID[step.ID] = step

		if executionErr != nil && step.Kind != StepKindCleanup {
			continue
		}

		resolvedRequest, requestErr := resolveStepRequest(step, stepOutputs)
		if requestErr != nil {
			stepErr := stepExecutionError(step, fmt.Errorf("resolve request: %w", requestErr))
			if step.Kind == StepKindCleanup {
				executionErr = errors.Join(executionErr, stepErr)
				continue
			}
			executionErr = stepErr
			continue
		}

		stepWithResolvedRequest := step
		stepWithResolvedRequest.Request = resolvedRequest

		stepOutput, stepErr := executeCaseStep(runContext, harness, stepWithResolvedRequest)
		if stepErr != nil {
			if step.Kind == StepKindCleanup {
				executionErr = errors.Join(executionErr, stepErr)
				continue
			}

			executionErr = stepErr
			continue
		}

		stepOutputs[step.ID] = stepOutput
		if step.Kind == StepKindAction {
			lastActionStepID = step.ID
		}
	}

	return assertCaseResult(
		testCase,
		executionErr,
		stepOutputs,
		stepsByID,
		lastActionStepID,
		inspector,
		statusCodec,
		config,
	)
}

// executeCaseStep executes one step of the case and returns its output.
func executeCaseStep[C any](runContext context.Context, harness C, step Step[C]) (any, error) {
	if step.Handler == nil {
		return nil, stepExecutionError(step, errors.New("handler is nil"))
	}

	if step.Kind == StepKindAwait {
		return executeAwaitStep(runContext, harness, step)
	}

	stepOutput, err := step.Handler.Invoke(runContext, harness, step.Request)
	if err != nil {
		return nil, stepExecutionError(step, err)
	}

	return stepOutput, nil
}

// executeAwaitStep executes an await step with a retry policy.
func executeAwaitStep[C any](runContext context.Context, harness C, step Step[C]) (any, error) {
	if step.Retry == nil {
		return nil, stepExecutionError(step, errors.New("retry policy is required for await step"))
	}

	retryPolicy := *step.Retry
	if retryPolicy.MaxAttempts <= 0 {
		return nil, buildAwaitRetryError(step, retryPolicy, 0, nil, nil, errors.New("max attempts reached"))
	}

	awaitContext, cancelAwait := context.WithTimeout(
		runContext,
		time.Duration(retryPolicy.TimeoutMS)*time.Millisecond,
	)
	defer cancelAwait()

	var (
		lastObservedOutput any
		lastObservedErr    error
		attemptsMade       int
	)

	operation := func() (any, error) {
		attemptsMade++
		stepOutput, invokeErr := step.Handler.Invoke(awaitContext, harness, step.Request)
		if invokeErr == nil {
			return stepOutput, nil
		}

		lastObservedOutput = stepOutput
		lastObservedErr = invokeErr
		if attemptsMade >= retryPolicy.MaxAttempts {
			return stepOutput, backoff.Permanent(invokeErr)
		}

		return stepOutput, invokeErr
	}

	retryBackoff := backoff.NewConstantBackOff(time.Duration(retryPolicy.IntervalMS) * time.Millisecond)
	result, retryErr := backoff.Retry(
		awaitContext,
		operation,
		backoff.WithBackOff(retryBackoff),
	)
	if retryErr == nil {
		return result, nil
	}

	if awaitContext.Err() != nil && attemptsMade < retryPolicy.MaxAttempts {
		finalOutput, finalErr := step.Handler.Invoke(awaitContext, harness, step.Request)
		attemptsMade++
		if finalErr == nil {
			return finalOutput, nil
		}

		if finalOutput != nil {
			lastObservedOutput = finalOutput
		}
		lastObservedErr = finalErr

		return nil, buildAwaitRetryError(
			step,
			retryPolicy,
			attemptsMade,
			lastObservedOutput,
			lastObservedErr,
			awaitContext.Err(),
		)
	}

	finalCause := awaitContext.Err()
	if finalCause == nil {
		finalCause = errors.New("max attempts reached")
	}

	return nil, buildAwaitRetryError(
		step,
		retryPolicy,
		attemptsMade,
		lastObservedOutput,
		lastObservedErr,
		finalCause,
	)
}

// stepExecutionError adds the step execution context to the error.
func stepExecutionError[C any](step Step[C], cause error) error {
	return fmt.Errorf("step.id=%s kind=%s handler=%s: %w", step.ID, step.Kind, step.HandlerName, cause)
}

// buildAwaitRetryError generates an await step retry policy exhaustion error.
func buildAwaitRetryError[C any](
	step Step[C],
	retryPolicy AwaitRetry,
	attemptsMade int,
	lastObservedOutput any,
	lastObservedErr error,
	cause error,
) error {
	details := fmt.Sprintf(
		"await retry exhausted: policy={timeout_ms=%d interval_ms=%d max_attempts=%d} attempts=%d",
		retryPolicy.TimeoutMS,
		retryPolicy.IntervalMS,
		retryPolicy.MaxAttempts,
		attemptsMade,
	)
	if cause != nil {
		details = fmt.Sprintf("%s cause=%v", details, cause)
	}
	if lastObservedErr != nil {
		details = fmt.Sprintf("%s last_error=%v", details, lastObservedErr)
	}
	if lastObservedOutput != nil {
		details = fmt.Sprintf("%s last_response=%v", details, lastObservedOutput)
	}

	wrappedErr := cause
	if lastObservedErr != nil {
		wrappedErr = lastObservedErr
	}
	if wrappedErr == nil {
		wrappedErr = errors.New("await retry exhausted")
	}

	return fmt.Errorf("step.id=%s kind=%s handler=%s: %s: %w", step.ID, step.Kind, step.HandlerName, details, wrappedErr)
}

// assertCaseResult checks the result of case execution according to the assert block.
func assertCaseResult[C any, S comparable](
	testCase Case[C, S],
	executionErr error,
	stepOutputs map[string]any,
	stepsByID map[string]Step[C],
	lastActionStepID string,
	inspector ErrorInspector[S],
	statusCodec StatusCodec[S],
	config runCasesConfig,
) error {
	if testCase.Assert.Code != statusCodec.Success() {
		return assertExpectedErrorResult(testCase.SourcePath, testCase.Assert, executionErr, inspector)
	}

	if executionErr != nil {
		return fmt.Errorf("%q: unexpected error: %w", testCase.SourcePath, executionErr)
	}
	return assertSuccessfulCaseResult(
		testCase,
		stepOutputs,
		stepsByID,
		lastActionStepID,
		config,
	)
}

// assertSuccessfulCaseResult checks the successful result of case execution.
func assertSuccessfulCaseResult[C any, S comparable](
	testCase Case[C, S],
	stepOutputs map[string]any,
	stepsByID map[string]Step[C],
	lastActionStepID string,
	config runCasesConfig,
) error {
	if testCase.Assert.Response == nil && testCase.Assert.ResponseMode != ResponseModePartial {
		return fmt.Errorf("%q: expected response is nil", testCase.SourcePath)
	}

	targetStepID := strings.TrimSpace(testCase.Assert.ResponseFromStep)
	if targetStepID == "" {
		targetStepID = lastActionStepID
	}
	targetStep, actualResponse, err := assertTargetResponse(
		testCase.SourcePath,
		targetStepID,
		stepsByID,
		stepOutputs,
	)
	if err != nil {
		return err
	}

	actualNormalized, err := targetStep.Handler.NormalizeResponse(actualResponse)
	if err != nil {
		return fmt.Errorf("%q: normalize actual response: %w", testCase.SourcePath, err)
	}
	if config.responseDumpCaseName != "" {
		return responseDumpError(actualNormalized)
	}

	matchErr := assertSuccessfulResponseMatch(testCase, targetStep, actualNormalized)
	if matchErr != nil {
		return matchErr
	}

	return assertSuccessMessageContains(testCase.SourcePath, testCase.Assert, actualNormalized)
}

// assertTargetResponse returns the assert target handler and the response of the selected step.
func assertTargetResponse[C any](
	sourcePath string,
	targetStepID string,
	stepsByID map[string]Step[C],
	stepOutputs map[string]any,
) (Step[C], any, error) {
	if targetStepID == "" {
		return Step[C]{}, nil, fmt.Errorf("%q: assert target step is not available", sourcePath)
	}

	targetStep, exists := stepsByID[targetStepID]
	if !exists {
		return Step[C]{}, nil, fmt.Errorf("%q: assert target step %q is not found", sourcePath, targetStepID)
	}
	if targetStep.Handler == nil {
		return Step[C]{}, nil, fmt.Errorf("%q: assert target step %q has nil handler", sourcePath, targetStepID)
	}

	actualResponse, exists := stepOutputs[targetStepID]
	if !exists {
		return Step[C]{}, nil, fmt.Errorf("%q: response from step %q is not available", sourcePath, targetStepID)
	}

	return targetStep, actualResponse, nil
}

// assertSuccessfulResponseMatch compares a successful response according to assert.response_mode.
func assertSuccessfulResponseMatch[C any, S comparable](
	testCase Case[C, S],
	targetStep Step[C],
	actualNormalized any,
) error {
	switch testCase.Assert.ResponseMode {
	case "", ResponseModeExact:
		expectedNormalized, normalizeErr := targetStep.Handler.NormalizeResponse(testCase.Assert.Response)
		if normalizeErr != nil {
			return fmt.Errorf("%q: normalize expected response: %w", testCase.SourcePath, normalizeErr)
		}
		return assertNormalizedResponseMatch(testCase.SourcePath, expectedNormalized, actualNormalized)
	case ResponseModePartial:
		return assertPartialResponseMatch(testCase.SourcePath, testCase.Assert.Response, actualNormalized)
	default:
		return fmt.Errorf(
			"%q: field assert.response_mode: unsupported value %q",
			testCase.SourcePath,
			testCase.Assert.ResponseMode,
		)
	}
}

// assertSuccessMessageContains tests a substring in a successful normalized response.
func assertSuccessMessageContains[S comparable](sourcePath string, assert Assert[S], actualNormalized any) error {
	if assert.MessageContains == "" {
		return nil
	}
	actualResponseText := fmt.Sprintf("%v", actualNormalized)
	if !strings.Contains(actualResponseText, assert.MessageContains) {
		return fmt.Errorf(
			"%q: expected successful response to contain %q",
			sourcePath,
			assert.MessageContains,
		)
	}
	return nil
}

// assertNormalizedResponseMatch tests the equality of normalized responses.
func assertNormalizedResponseMatch(sourcePath string, expectedNormalized, actualNormalized any) error {
	responsesEqual, responseDiff, err := compareNormalizedResponses(expectedNormalized, actualNormalized)
	if err != nil {
		return fmt.Errorf("%q: response comparison failed: %w", sourcePath, err)
	}
	if !responsesEqual {
		return responseMismatchError(sourcePath, responseDiff)
	}
	return nil
}

// assertPartialResponseMatch checks for the presence of expected fields in the normalized response.
func assertPartialResponseMatch(sourcePath string, expectedPartial, actualNormalized any) error {
	if err := comparePartialResponseValue("$", expectedPartial, actualNormalized); err != nil {
		return fmt.Errorf("%q: response partial mismatch: %w", sourcePath, err)
	}
	return nil
}

// comparePartialResponseValue recursively compares JSON-like response values ​​in `partial` mode.
func comparePartialResponseValue(path string, expected, actual any) error {
	switch expectedValue := expected.(type) {
	case map[string]any:
		actualValue, ok := actual.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: type mismatch: expected object %v, actual %v (%T)", path, expected, actual, actual)
		}
		for key, nestedExpected := range expectedValue {
			nestedPath := partialResponsePath(path, key)
			nestedActual, exists := actualValue[key]
			if !exists {
				return fmt.Errorf(
					"%s: missing field: expected %v (%T), actual <missing>",
					nestedPath,
					nestedExpected,
					nestedExpected,
				)
			}
			if err := comparePartialResponseValue(nestedPath, nestedExpected, nestedActual); err != nil {
				return err
			}
		}
		return nil
	case []any:
		actualValue, ok := actual.([]any)
		if !ok {
			return fmt.Errorf("%s: type mismatch: expected array %v, actual %v (%T)", path, expected, actual, actual)
		}
		if len(expectedValue) != len(actualValue) {
			return fmt.Errorf("%s: array length mismatch: expected %d, actual %d", path, len(expectedValue), len(actualValue))
		}
		for index, nestedExpected := range expectedValue {
			nestedPath := fmt.Sprintf("%s[%d]", path, index)
			if err := comparePartialResponseValue(nestedPath, nestedExpected, actualValue[index]); err != nil {
				return err
			}
		}
		return nil
	default:
		if !reflect.DeepEqual(expected, actual) {
			return fmt.Errorf("%s: value mismatch: expected %v (%T), actual %v (%T)", path, expected, expected, actual, actual)
		}
		return nil
	}
}

// partialResponsePath appends the field name to the JSON-like path.
func partialResponsePath(parentPath, fieldName string) string {
	if parentPath == "$" {
		return "$" + "." + fieldName
	}
	return parentPath + "." + fieldName
}

// protoCompareVisit stores an already processed pointer to protect against loops.
type protoCompareVisit struct {
	valueType reflect.Type
	pointer   uintptr
}

// compareNormalizedResponses compares normalized responses and returns the differences if there is a mismatch.
func compareNormalizedResponses(expected, actual any) (equal bool, diff string, err error) {
	if !containsProtoMessage(expected) && !containsProtoMessage(actual) {
		if reflect.DeepEqual(expected, actual) {
			return true, "", nil
		}

		diff, err = safeCmpDiff(expected, actual)
		if err != nil {
			return false, "", err
		}
		return false, diff, nil
	}

	expectedView, err := normalizeProtoMessagesInValue(expected)
	if err != nil {
		return false, "", fmt.Errorf("normalize expected response protobuf values: %w", err)
	}
	actualView, err := normalizeProtoMessagesInValue(actual)
	if err != nil {
		return false, "", fmt.Errorf("normalize actual response protobuf values: %w", err)
	}
	if reflect.DeepEqual(expectedView, actualView) {
		return true, "", nil
	}

	diff, err = jsonLikeDiff(expectedView, actualView)
	if err != nil {
		return false, "", err
	}
	return false, diff, nil
}

// jsonLikeDiff builds human-readable diff output for JSON-like values.
func jsonLikeDiff(expected, actual any) (string, error) {
	expectedJSON, err := encodeIndentedJSON(expected)
	if err != nil {
		return "", fmt.Errorf("encode expected response JSON view: %w", err)
	}
	actualJSON, err := encodeIndentedJSON(actual)
	if err != nil {
		return "", fmt.Errorf("encode actual response JSON view: %w", err)
	}

	diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(expectedJSON),
		FromFile: "expected",
		FromDate: "",
		B:        difflib.SplitLines(actualJSON),
		ToFile:   "actual",
		ToDate:   "",
		Eol:      "",
		Context:  DefaultJSONDiffContextLines,
	})
	if err != nil {
		return "", fmt.Errorf("build response JSON diff: %w", err)
	}
	return diff, nil
}

// normalizeProtoMessagesInValue converts protobuf values ​​to JSON-like form for comparison.
func normalizeProtoMessagesInValue(value any) (any, error) {
	return normalizeProtoMessagesInReflectValue(reflect.ValueOf(value), make(map[protoCompareVisit]bool))
}

// normalizeProtoMessagesInReflectValue recursively constructs a JSON-like value without internal protobuf fields.
func normalizeProtoMessagesInReflectValue(value reflect.Value, visited map[protoCompareVisit]bool) (any, error) {
	if !value.IsValid() {
		return nilNormalizedValue(), nil
	}
	if isProtoMessageValue(value) {
		return normalizeProtoMessageValue(value)
	}

	kind := value.Kind()
	if kind == reflect.Interface {
		return normalizeProtoMessagesInInterface(value, visited)
	}
	if kind == reflect.Pointer {
		return normalizeProtoMessagesInPointer(value, visited)
	}
	if kind == reflect.Slice || kind == reflect.Array {
		return normalizeProtoMessagesInIndexed(value, visited)
	}
	if kind == reflect.Map {
		return normalizeProtoMessagesInMap(value, visited)
	}
	if kind == reflect.Struct {
		return normalizeProtoMessagesInStruct(value, visited)
	}
	if !value.CanInterface() {
		return nilNormalizedValue(), nil
	}
	return value.Interface(), nil
}

// normalizeProtoMessageValue converts a single proto.Message to a JSON-like value.
func normalizeProtoMessageValue(value reflect.Value) (any, error) {
	message, ok := protoMessageFromValue(value)
	if !ok {
		return nil, fmt.Errorf("value %s does not implement proto.Message", value.Type())
	}
	if isNilValue(value) {
		return nilNormalizedValue(), nil
	}
	return protojsonview.NormalizeMessage(message)
}

// normalizeProtoMessagesInInterface expands the interface value.
func normalizeProtoMessagesInInterface(value reflect.Value, visited map[protoCompareVisit]bool) (any, error) {
	if value.IsNil() {
		return nilNormalizedValue(), nil
	}
	return normalizeProtoMessagesInReflectValue(value.Elem(), visited)
}

// normalizeProtoMessagesInPointer expands the pointer and protects the traversal from loops.
func normalizeProtoMessagesInPointer(value reflect.Value, visited map[protoCompareVisit]bool) (any, error) {
	if value.IsNil() {
		return nilNormalizedValue(), nil
	}

	visit := protoCompareVisit{
		valueType: value.Type(),
		pointer:   value.Pointer(),
	}
	if visited[visit] {
		return nil, fmt.Errorf("cyclic normalized response at %s", value.Type())
	}
	visited[visit] = true
	defer delete(visited, visit)

	return normalizeProtoMessagesInReflectValue(value.Elem(), visited)
}

// normalizeProtoMessagesInIndexed converts an array or slice to a JSON-like slice.
func normalizeProtoMessagesInIndexed(value reflect.Value, visited map[protoCompareVisit]bool) (any, error) {
	if value.Kind() == reflect.Slice && value.IsNil() {
		return nilNormalizedValue(), nil
	}

	items := make([]any, 0, value.Len())
	for index := range value.Len() {
		item, err := normalizeProtoMessagesInReflectValue(value.Index(index), visited)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// normalizeProtoMessagesInMap converts the map to a JSON-like value.
func normalizeProtoMessagesInMap(value reflect.Value, visited map[protoCompareVisit]bool) (any, error) {
	if value.IsNil() {
		return nilNormalizedValue(), nil
	}
	if value.Type().Key().Kind() == reflect.String {
		return normalizeProtoMessagesInStringMap(value, visited)
	}
	return normalizeProtoMessagesInAnyMap(value, visited)
}

// normalizeProtoMessagesInStringMap saves the map's string keys.
func normalizeProtoMessagesInStringMap(
	value reflect.Value,
	visited map[protoCompareVisit]bool,
) (map[string]any, error) {
	items := make(map[string]any, value.Len())
	for _, key := range value.MapKeys() {
		item, err := normalizeProtoMessagesInReflectValue(value.MapIndex(key), visited)
		if err != nil {
			return nil, err
		}
		items[key.String()] = item
	}
	return items, nil
}

// normalizeProtoMessagesInAnyMap converts non-string map keys to strings for the JSON representation.
func normalizeProtoMessagesInAnyMap(value reflect.Value, visited map[protoCompareVisit]bool) (map[string]any, error) {
	items := make(map[string]any, value.Len())
	for _, key := range value.MapKeys() {
		if !key.CanInterface() {
			return nil, fmt.Errorf("map key %s cannot be converted to interface", key.Type())
		}
		item, err := normalizeProtoMessagesInReflectValue(value.MapIndex(key), visited)
		if err != nil {
			return nil, err
		}
		items[fmt.Sprint(key.Interface())] = item
	}
	return items, nil
}

// normalizeProtoMessagesInStruct converts the exported fields of the structure to a JSON-like map.
func normalizeProtoMessagesInStruct(value reflect.Value, visited map[protoCompareVisit]bool) (map[string]any, error) {
	fields := make(map[string]any, value.NumField())
	for index := range value.NumField() {
		field := value.Type().Field(index)
		if !field.IsExported() {
			continue
		}

		fieldValue, err := normalizeProtoMessagesInReflectValue(value.Field(index), visited)
		if err != nil {
			return nil, err
		}
		fields[field.Name] = fieldValue
	}
	return fields, nil
}

// nilNormalizedValue returns a JSON-like nil without nilnil triggering.
func nilNormalizedValue() any {
	return nil
}

// containsProtoMessage checks for the presence of proto.Message in the values.
func containsProtoMessage(values ...any) bool {
	for _, value := range values {
		if containsProtoMessageValue(reflect.ValueOf(value), make(map[protoCompareVisit]bool)) {
			return true
		}
	}
	return false
}

// containsProtoMessageValue recursively looks up proto.Message without expanding the proto messages themselves.
func containsProtoMessageValue(value reflect.Value, visited map[protoCompareVisit]bool) bool {
	if !value.IsValid() {
		return false
	}
	if isProtoMessageValue(value) {
		return true
	}

	kind := value.Kind()
	if kind == reflect.Interface {
		return !value.IsNil() && containsProtoMessageValue(value.Elem(), visited)
	}
	if kind == reflect.Pointer {
		return containsProtoMessageInPointer(value, visited)
	}
	if kind == reflect.Slice || kind == reflect.Array {
		return containsProtoMessageInIndexed(value, visited)
	}
	if kind == reflect.Map {
		return containsProtoMessageInMap(value, visited)
	}
	if kind == reflect.Struct {
		return containsProtoMessageInStruct(value, visited)
	}
	return false
}

// containsProtoMessageInPointer checks the value under the pointer without protobuf.
func containsProtoMessageInPointer(value reflect.Value, visited map[protoCompareVisit]bool) bool {
	if value.IsNil() {
		return false
	}
	visit := protoCompareVisit{
		valueType: value.Type(),
		pointer:   value.Pointer(),
	}
	if visited[visit] {
		return false
	}
	visited[visit] = true
	defer delete(visited, visit)
	return containsProtoMessageValue(value.Elem(), visited)
}

// containsProtoMessageInIndexed checks the elements of an array or slice.
func containsProtoMessageInIndexed(value reflect.Value, visited map[protoCompareVisit]bool) bool {
	for index := range value.Len() {
		if containsProtoMessageValue(value.Index(index), visited) {
			return true
		}
	}
	return false
}

// containsProtoMessageInMap checks the keys and values ​​of the map.
func containsProtoMessageInMap(value reflect.Value, visited map[protoCompareVisit]bool) bool {
	for _, key := range value.MapKeys() {
		if containsProtoMessageValue(key, visited) || containsProtoMessageValue(value.MapIndex(key), visited) {
			return true
		}
	}
	return false
}

// containsProtoMessageInStruct checks the fields of a structure.
func containsProtoMessageInStruct(value reflect.Value, visited map[protoCompareVisit]bool) bool {
	for _, field := range value.Fields() {
		if containsProtoMessageValue(field, visited) {
			return true
		}
	}
	return false
}

// isProtoMessageValue checks whether the value implements proto.Message.
func isProtoMessageValue(value reflect.Value) bool {
	return value.IsValid() && value.Type().Implements(reflect.TypeFor[proto.Message]())
}

// protoMessageFromValue returns proto.Message from reflect.Value.
func protoMessageFromValue(value reflect.Value) (proto.Message, bool) {
	if !isProtoMessageValue(value) || !value.CanInterface() {
		return nil, false
	}
	message, ok := value.Interface().(proto.Message)
	return message, ok
}

// isNilValue checks for nil for types that can be nil.
func isNilValue(value reflect.Value) bool {
	kind := value.Kind()
	if kind == reflect.Chan || kind == reflect.Func || kind == reflect.Interface || kind == reflect.Map ||
		kind == reflect.Pointer || kind == reflect.Slice {
		return value.IsNil()
	}
	return false
}

// responseMismatchError generates a response mismatch error.
func responseMismatchError(sourcePath, diff string) error {
	if diff == "" {
		return fmt.Errorf("%q: response mismatch", sourcePath)
	}

	return fmt.Errorf("%q: response mismatch (-want +got):\n%s", sourcePath, diff)
}

// responseDumpError returns a JSON dump of the actual response as the only error content.
func responseDumpError(actualNormalized any) error {
	actualDump := actualNormalized
	var err error
	if containsProtoMessage(actualNormalized) {
		actualDump, err = normalizeProtoMessagesInValue(actualNormalized)
		if err != nil {
			return fmt.Errorf("response dump failed: %w", err)
		}
	}

	actualResponseJSON, err := encodeIndentedJSON(actualDump)
	if err != nil {
		return fmt.Errorf("response dump failed: %w", err)
	}

	return errors.New("ITESTKIT_RESPONSE_DUMP_BEGIN\n" + actualResponseJSON + "\nITESTKIT_RESPONSE_DUMP_END")
}

// encodeIndentedJSON encodes the value into formatted JSON without HTML escaping.
func encodeIndentedJSON(value any) (string, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(value); err != nil {
		return "", err
	}

	return strings.TrimRight(buffer.String(), "\n"), nil
}

// assertExpectedErrorResult tests the assert branch for an expected error.
func assertExpectedErrorResult[S comparable](
	sourcePath string,
	assertSection Assert[S],
	executionErr error,
	inspector ErrorInspector[S],
) error {
	if executionErr == nil {
		return fmt.Errorf(
			"%q: expected error with code %v, got nil",
			sourcePath,
			assertSection.Code,
		)
	}
	if inspector == nil {
		return fmt.Errorf("%q: error inspector is nil", sourcePath)
	}

	code, message, ok := inspector.FromError(executionErr)
	if !ok {
		return fmt.Errorf("%q: failed to inspect error", sourcePath)
	}
	if code != assertSection.Code {
		return fmt.Errorf(
			"%q: expected error code %v, got %v",
			sourcePath,
			assertSection.Code,
			code,
		)
	}
	if assertSection.MessageContains != "" && !strings.Contains(message, assertSection.MessageContains) {
		return fmt.Errorf(
			"%q: expected error message to contain %q",
			sourcePath,
			assertSection.MessageContains,
		)
	}

	return nil
}

// safeCmpDiff returns diff from go-cmp.
//
// cmp.Diff may panic on some types (for example, if there are non-exported fields),
// so here we play it safe and return an error instead of panic.
func safeCmpDiff(expected, actual any, options ...cmp.Option) (diff string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("cmp diff panic: %v", r)
		}
	}()

	return cmp.Diff(expected, actual, options...), nil
}
