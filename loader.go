package itestkit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
)

// LoadCases loads and validates cases from the source.
func LoadCases[C any, S comparable](
	source CaseSource,
	rootDir string,
	registry HandlerRegistry[C],
	statusCodec StatusCodec[S],
) ([]Case[C, S], error) {
	casePaths, err := listCasePaths(source, rootDir)
	if err != nil {
		return nil, fmt.Errorf("list test cases: %w", err)
	}
	if len(casePaths) == 0 {
		return nil, fmt.Errorf("no .jsonc cases found in %s", rootDir)
	}

	loadedCases := make([]Case[C, S], 0, len(casePaths))
	seenNames := make(map[string]string, len(casePaths))
	for _, casePath := range casePaths {
		loadedCase, loadErr := loadCase(source, casePath, registry, statusCodec)
		if loadErr != nil {
			return nil, fmt.Errorf("load %s: %w", casePath, loadErr)
		}

		if validateErr := validateUniqueCaseName(seenNames, loadedCase.Name, casePath); validateErr != nil {
			return nil, validateErr
		}

		seenNames[loadedCase.Name] = casePath
		loadedCases = append(loadedCases, loadedCase)
	}

	return loadedCases, nil
}

// validateUniqueCaseName checks the uniqueness of the case name among previously loaded ones.
func validateUniqueCaseName(seenNames map[string]string, caseName, casePath string) error {
	if previousPath, duplicated := seenNames[caseName]; duplicated {
		return fmt.Errorf("duplicate case name %q: %s and %s", caseName, previousPath, casePath)
	}

	return nil
}

// listCasePaths returns the sorted paths of .jsonc files recursively.
func listCasePaths(source CaseSource, rootDir string) ([]string, error) {
	entries, err := source.ReadDir(rootDir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", rootDir, err)
	}

	casePaths := make([]string, 0, len(entries))
	for _, entry := range entries {
		entryPath := path.Join(rootDir, entry.Name())
		if entry.IsDir() {
			nestedPaths, nestedErr := listCasePaths(source, entryPath)
			if nestedErr != nil {
				return nil, nestedErr
			}
			casePaths = append(casePaths, nestedPaths...)
			continue
		}

		if path.Ext(entry.Name()) == ".jsonc" {
			casePaths = append(casePaths, entryPath)
		}
	}

	sort.Strings(casePaths)

	return casePaths, nil
}

// loadCase loads a single JSONC case and converts it into a typed structure.
func loadCase[C any, S comparable](
	source CaseSource,
	casePath string,
	registry HandlerRegistry[C],
	statusCodec StatusCodec[S],
) (Case[C, S], error) {
	rawCase, err := source.ReadFile(casePath)
	if err != nil {
		return Case[C, S]{}, fmt.Errorf("read file: %w", err)
	}

	jsonCase, err := stripJSONCComments(rawCase)
	if err != nil {
		return Case[C, S]{}, fmt.Errorf("strip comments: %w", err)
	}

	fileCase, err := decodeCaseFile(jsonCase)
	if err != nil {
		return Case[C, S]{}, fmt.Errorf("decode case file: %w", err)
	}

	return mapCaseFile(casePath, fileCase, registry, statusCodec)
}

// decodeCaseFile decodes JSON and disables unknown fields.
func decodeCaseFile(rawJSON []byte) (caseFile, error) {
	decoder := json.NewDecoder(bytes.NewReader(rawJSON))
	decoder.DisallowUnknownFields()

	var decoded caseFile
	if err := decoder.Decode(&decoded); err != nil {
		return caseFile{}, fmt.Errorf("decode: %w", err)
	}

	var trailing any
	err := decoder.Decode(&trailing)
	if err != io.EOF {
		if err == nil {
			return caseFile{}, errors.New("unexpected trailing JSON token")
		}
		return caseFile{}, fmt.Errorf("unexpected trailing JSON: %w", err)
	}

	return decoded, nil
}

// mapCaseFile validates required fields and parses payload.
func mapCaseFile[C any, S comparable](
	casePath string,
	fileCase caseFile,
	registry HandlerRegistry[C],
	statusCodec StatusCodec[S],
) (Case[C, S], error) {
	caseName := strings.TrimSpace(fileCase.Name)
	if caseName == "" {
		return Case[C, S]{}, fmt.Errorf("%s: field name is required", casePath)
	}

	if len(fileCase.Steps) == 0 {
		return Case[C, S]{}, fmt.Errorf("%s: field steps is required", casePath)
	}

	loadedSteps := make([]Step[C], 0, len(fileCase.Steps))
	stepsByID := make(map[string]Step[C], len(fileCase.Steps))
	lastActionStepID := ""
	for index, fileStep := range fileCase.Steps {
		fieldPath := fmt.Sprintf("steps[%d]", index)

		loadedStep, err := mapStep(casePath, fieldPath, fileStep, registry)
		if err != nil {
			return Case[C, S]{}, err
		}

		if _, duplicated := stepsByID[loadedStep.ID]; duplicated {
			return Case[C, S]{}, fmt.Errorf(
				"%s: field %s.id: duplicate step id %q",
				casePath,
				fieldPath,
				loadedStep.ID,
			)
		}

		stepsByID[loadedStep.ID] = loadedStep
		if loadedStep.Kind == StepKindAction {
			lastActionStepID = loadedStep.ID
		}

		loadedSteps = append(loadedSteps, loadedStep)
	}

	assertSection, err := mapAssertSection(casePath, fileCase.Assert, statusCodec, stepsByID, lastActionStepID)
	if err != nil {
		return Case[C, S]{}, err
	}

	return Case[C, S]{
		Name:       caseName,
		SourcePath: casePath,
		Steps:      loadedSteps,
		Assert:     assertSection,
	}, nil
}

// mapStep validates the handler, decodes the request and returns the finished step.
func mapStep[C any](
	casePath string,
	fieldPath string,
	rawStep stepFile,
	registry HandlerRegistry[C],
) (Step[C], error) {
	stepID := strings.TrimSpace(rawStep.ID)
	if stepID == "" {
		return Step[C]{}, fmt.Errorf("%s: field %s.id is required", casePath, fieldPath)
	}

	kind, err := parseStepKind(casePath, fieldPath, rawStep.Kind)
	if err != nil {
		return Step[C]{}, err
	}

	normalizedHandler := strings.TrimSpace(rawStep.Handler)
	if normalizedHandler == "" {
		return Step[C]{}, fmt.Errorf("%s: field %s.handler is required", casePath, fieldPath)
	}

	handler, err := registry.Resolve(normalizedHandler)
	if err != nil {
		supported := registry.Supported()
		sort.Strings(supported)
		return Step[C]{}, fmt.Errorf(
			"%s: field %s.handler: unsupported handler %q; supported handlers: %s",
			casePath,
			fieldPath,
			normalizedHandler,
			strings.Join(supported, ", "),
		)
	}

	rawRequest := rawStep.Request
	if len(bytes.TrimSpace(rawRequest)) == 0 {
		return Step[C]{}, fmt.Errorf("%s: field %s.request is required", casePath, fieldPath)
	}

	var request any
	// Dynamic templates are resolved at runtime from the output of previous steps,
	// therefore, the decode request must be deferred until the execution phase.
	if requestContainsStepTemplate(rawRequest) {
		request = stepDynamicRequest{raw: bytes.Clone(rawRequest)}
	} else {
		request, err = handler.DecodeRequest(rawRequest)
		if err != nil {
			return Step[C]{}, fmt.Errorf("%s: decode %s.request: %w", casePath, fieldPath, err)
		}
	}

	retryValue, hasRetry, err := mapAwaitRetry(casePath, fieldPath, kind, rawStep.Retry)
	if err != nil {
		return Step[C]{}, err
	}

	var retry *AwaitRetry
	if hasRetry {
		retry = &retryValue
	}

	return Step[C]{
		ID:          stepID,
		Kind:        kind,
		HandlerName: normalizedHandler,
		Handler:     handler,
		Request:     request,
		Retry:       retry,
	}, nil
}

// parseStepKind validates the kind of a step and converts it into a domain model.
func parseStepKind(casePath, fieldPath string, rawKind stepKindFile) (StepKind, error) {
	normalized := strings.TrimSpace(string(rawKind))
	if normalized == "" {
		return "", fmt.Errorf("%s: field %s.kind is required", casePath, fieldPath)
	}

	switch StepKind(normalized) {
	case StepKindPrepare, StepKindAction, StepKindPublish, StepKindAwait, StepKindVerify, StepKindCleanup:
		return StepKind(normalized), nil
	default:
		return "", fmt.Errorf(
			"%s: field %s.kind: unsupported kind %q; allowed kinds: prepare, action, publish, await, verify, cleanup",
			casePath,
			fieldPath,
			normalized,
		)
	}
}

// mapAwaitRetry validates the retry policy of the await step.
func mapAwaitRetry(
	casePath string,
	fieldPath string,
	kind StepKind,
	retryFile *awaitRetryFile,
) (AwaitRetry, bool, error) {
	emptyRetry := AwaitRetry{
		TimeoutMS:   0,
		IntervalMS:  0,
		MaxAttempts: 0,
	}

	if kind != StepKindAwait {
		if retryFile != nil {
			return emptyRetry, false, fmt.Errorf("%s: field %s.retry is allowed only for kind=await", casePath, fieldPath)
		}
		return emptyRetry, false, nil
	}

	if retryFile == nil {
		return emptyRetry, false, fmt.Errorf("%s: field %s.retry is required for kind=await", casePath, fieldPath)
	}

	if retryFile.TimeoutMS <= 0 {
		return emptyRetry, false, fmt.Errorf("%s: field %s.retry.timeout_ms must be > 0", casePath, fieldPath)
	}
	if retryFile.IntervalMS <= 0 {
		return emptyRetry, false, fmt.Errorf("%s: field %s.retry.interval_ms must be > 0", casePath, fieldPath)
	}
	if retryFile.MaxAttempts <= 0 {
		return emptyRetry, false, fmt.Errorf("%s: field %s.retry.max_attempts must be > 0", casePath, fieldPath)
	}

	return AwaitRetry{
		TimeoutMS:   retryFile.TimeoutMS,
		IntervalMS:  retryFile.IntervalMS,
		MaxAttempts: retryFile.MaxAttempts,
	}, true, nil
}

// mapAssertSection parses the assert block and validates the link to the response source.
func mapAssertSection[C any, S comparable](
	casePath string,
	assertCase assertFileSection,
	statusCodec StatusCodec[S],
	stepsByID map[string]Step[C],
	lastActionStepID string,
) (Assert[S], error) {
	code, err := statusCodec.Parse(assertCase.Code)
	if err != nil {
		return Assert[S]{}, fmt.Errorf("%s: field assert.code: %w", casePath, err)
	}

	responseFromStep := strings.TrimSpace(assertCase.ResponseFromStep)
	targetStep, hasTargetStep := stepsByID[responseFromStep]
	if responseFromStep != "" && !hasTargetStep {
		return Assert[S]{}, fmt.Errorf(
			"%s: field assert.response_from_step: unknown step id %q",
			casePath,
			responseFromStep,
		)
	}

	responseProvided := len(bytes.TrimSpace(assertCase.Response)) > 0
	if responseProvided && responseFromStep == "" && lastActionStepID == "" {
		return Assert[S]{}, fmt.Errorf(
			"%s: field assert.response requires at least one action step when assert.response_from_step is empty",
			casePath,
		)
	}

	if responseFromStep == "" && lastActionStepID != "" {
		targetStep = stepsByID[lastActionStepID]
		hasTargetStep = true
	}

	responseMode, err := parseResponseMode(assertCase.ResponseMode)
	if err != nil {
		return Assert[S]{}, fmt.Errorf("%s: field assert.response_mode: %w", casePath, err)
	}

	loadedAssert := Assert[S]{
		Code:             code,
		MessageContains:  assertCase.MessageContains,
		Response:         nil,
		ResponseFromStep: responseFromStep,
		ResponseMode:     responseMode,
	}
	if code != statusCodec.Success() {
		if assertCase.ResponseMode != "" {
			return Assert[S]{}, fmt.Errorf(
				"%s: field assert.response_mode requires code=%v",
				casePath,
				statusCodec.Success(),
			)
		}
		return loadedAssert, nil
	}

	expectedResponse, err := mapSuccessAssertResponse(
		casePath,
		assertCase.Response,
		responseProvided,
		hasTargetStep,
		targetStep,
		responseMode,
		statusCodec.Success(),
	)
	if err != nil {
		return Assert[S]{}, err
	}
	loadedAssert.Response = expectedResponse

	return loadedAssert, nil
}

// mapSuccessAssertResponse loads the expected response for a successful assert script.
func mapSuccessAssertResponse[C any, S comparable](
	casePath string,
	rawResponse json.RawMessage,
	responseProvided bool,
	hasTargetStep bool,
	targetStep Step[C],
	responseMode ResponseMode,
	successCode S,
) (any, error) {
	if !responseProvided {
		return nil, fmt.Errorf(
			"%s: field assert.response is required for code=%v",
			casePath,
			successCode,
		)
	}
	if !hasTargetStep {
		return nil, fmt.Errorf(
			"%s: field assert.response cannot be decoded because no target step is available",
			casePath,
		)
	}

	if responseMode == ResponseModePartial {
		expectedResponse, decodeErr := DecodeExpectedJSON(rawResponse)
		if decodeErr != nil {
			return nil, fmt.Errorf("%s: decode raw assert.response: %w", casePath, decodeErr)
		}
		return expectedResponse, nil
	}

	rawExpectedResponse, rawDecodeErr := decodeRawExpectedResponse(rawResponse)
	if rawDecodeErr != nil {
		return nil, fmt.Errorf("%s: decode raw assert.response: %w", casePath, rawDecodeErr)
	}
	if responseTemplateContainsSpecialMatcher(rawExpectedResponse) {
		return rawExpectedResponse, nil
	}

	expectedResponse, decodeErr := targetStep.Handler.DecodeExpectedResponse(rawResponse)
	if decodeErr != nil {
		return nil, fmt.Errorf("%s: decode assert.response: %w", casePath, decodeErr)
	}

	return expectedResponse, nil
}

// decodeRawExpectedResponse decodes a partial response without losing the JSON numeric type.
func decodeRawExpectedResponse(raw json.RawMessage) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	var result any
	if err := decoder.Decode(&result); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("trailing data")
	}

	return result, nil
}

// parseResponseMode returns the response comparison mode for the assert block.
func parseResponseMode(raw ResponseMode) (ResponseMode, error) {
	switch raw {
	case "", ResponseModeExact:
		return ResponseModeExact, nil
	case ResponseModePartial:
		return ResponseModePartial, nil
	default:
		return "", fmt.Errorf(
			"unsupported value %q; allowed values are %q and %q",
			raw,
			ResponseModeExact,
			ResponseModePartial,
		)
	}
}

// jsoncParserState describes the current state of the JSONC parser.
type jsoncParserState int

const (
	jsoncStateCode jsoncParserState = iota
	jsoncStateString
	jsoncStateLineComment
	jsoncStateBlockComment
)

// stripJSONCComments removes comments// and /* */ from JSONC.
func stripJSONCComments(source []byte) ([]byte, error) {
	var (
		state   = jsoncStateCode
		escaped bool
		result  bytes.Buffer
	)

	for index := 0; index < len(source); index++ {
		current := source[index]
		next, hasNext := peekJSONCByte(source, index)

		switch state {
		case jsoncStateString:
			state, escaped = handleStringState(current, escaped, &result)
		case jsoncStateLineComment:
			state = handleLineCommentState(current, &result)
		case jsoncStateBlockComment:
			var advance bool
			state, advance = handleBlockCommentState(current, next, hasNext, &result)
			if advance {
				index++
			}
		case jsoncStateCode:
			var advance bool
			state, advance = handleCodeState(current, next, hasNext, &result)
			if advance {
				index++
			}
		default:
			state = jsoncStateCode
		}
	}

	if state == jsoncStateBlockComment {
		return nil, errors.New("unterminated block comment")
	}

	return result.Bytes(), nil
}

// peekJSONCByte returns the next byte and the availability flag.
func peekJSONCByte(source []byte, index int) (byte, bool) {
	if index+1 >= len(source) {
		return 0, false
	}
	return source[index+1], true
}

// handleStringState handles a byte inside a string literal.
func handleStringState(current byte, escaped bool, result *bytes.Buffer) (jsoncParserState, bool) {
	result.WriteByte(current)
	if escaped {
		return jsoncStateString, false
	}
	if current == '\\' {
		return jsoncStateString, true
	}
	if current == '"' {
		return jsoncStateCode, false
	}
	return jsoncStateString, false
}

// handleLineCommentState handles the byte inside a single line comment.
func handleLineCommentState(current byte, result *bytes.Buffer) jsoncParserState {
	if current == '\n' {
		result.WriteByte('\n')
		return jsoncStateCode
	}
	return jsoncStateLineComment
}

// handleBlockCommentState handles the byte inside a multiline comment.
func handleBlockCommentState(
	current byte,
	next byte,
	hasNext bool,
	result *bytes.Buffer,
) (jsoncParserState, bool) {
	if current == '\n' {
		result.WriteByte('\n')
		return jsoncStateBlockComment, false
	}
	if current == '*' && hasNext && next == '/' {
		return jsoncStateCode, true
	}
	return jsoncStateBlockComment, false
}

// handleCodeState handles the byte in normal code.
func handleCodeState(
	current byte,
	next byte,
	hasNext bool,
	result *bytes.Buffer,
) (jsoncParserState, bool) {
	if current == '"' {
		result.WriteByte(current)
		return jsoncStateString, false
	}
	if current == '/' && hasNext {
		if next == '/' {
			return jsoncStateLineComment, true
		}
		if next == '*' {
			return jsoncStateBlockComment, true
		}
	}

	result.WriteByte(current)
	return jsoncStateCode, false
}
