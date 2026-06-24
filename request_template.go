package itestkit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

const (
	templatePrefix          = "{{"
	templateSuffix          = "}}"
	templateStepsPrefix     = "steps."
	templateResponseSegment = "response"
	templateMinParts        = 2
	templatePathOffset      = 2
)

// stepDynamicRequest stores the raw request JSON with templates for links to the step output.
type stepDynamicRequest struct {
	raw json.RawMessage
}

// stepTemplateReference describes the parsed template expression:
// {{steps.<step-id>.response[.<path>...]}}.
type stepTemplateReference struct {
	stepID string
	path   []string
}

// requestContainsStepTemplate performs a quick preliminary check for the presence of templates.
func requestContainsStepTemplate(raw json.RawMessage) bool {
	return bytes.Contains(raw, []byte(templatePrefix+templateStepsPrefix))
}

// resolveStepRequest returns a request ready to be executed:
// 1) the static request is returned as is;
// 2) the dynamic request is resolved from the output of the previous steps and decoded by the handler.
func resolveStepRequest[C any](step Step[C], stepOutputs map[string]any) (any, error) {
	dynamicRequest, hasTemplate := step.Request.(stepDynamicRequest)
	if !hasTemplate {
		return step.Request, nil
	}
	if step.Handler == nil {
		return nil, errors.New("handler is nil")
	}

	resolvedRaw, err := resolveStepRequestTemplate(dynamicRequest.raw, stepOutputs)
	if err != nil {
		return nil, err
	}

	request, err := step.Handler.DecodeRequest(resolvedRaw)
	if err != nil {
		return nil, fmt.Errorf("decode dynamic request: %w", err)
	}

	return request, nil
}

// resolveStepRequestTemplate applies template substitutions to request JSON.
func resolveStepRequestTemplate(raw json.RawMessage, stepOutputs map[string]any) (json.RawMessage, error) {
	var payload any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode request template: %w", err)
	}

	resolvedPayload, err := resolveTemplateNode(payload, stepOutputs)
	if err != nil {
		return nil, err
	}

	resolvedRaw, err := json.Marshal(resolvedPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal resolved request template: %w", err)
	}

	return resolvedRaw, nil
}

// resolveTemplateNode recursively traverses the request payload and replaces the template strings.
func resolveTemplateNode(node any, stepOutputs map[string]any) (any, error) {
	switch typedNode := node.(type) {
	case string:
		reference, isTemplate, err := parseStepTemplateReference(typedNode)
		if err != nil {
			return nil, err
		}
		if !isTemplate {
			return typedNode, nil
		}

		resolvedValue, err := resolveTemplateReference(reference, stepOutputs)
		if err != nil {
			return nil, err
		}

		return resolvedValue, nil
	case []any:
		resolvedSlice := make([]any, 0, len(typedNode))
		for _, item := range typedNode {
			resolvedItem, err := resolveTemplateNode(item, stepOutputs)
			if err != nil {
				return nil, err
			}
			resolvedSlice = append(resolvedSlice, resolvedItem)
		}

		return resolvedSlice, nil
	case map[string]any:
		resolvedMap := make(map[string]any, len(typedNode))
		for key, value := range typedNode {
			resolvedValue, err := resolveTemplateNode(value, stepOutputs)
			if err != nil {
				return nil, err
			}
			resolvedMap[key] = resolvedValue
		}

		return resolvedMap, nil
	default:
		return node, nil
	}
}

// parseStepTemplateReference parses the supported template syntax:
// {{steps.<step-id>.response}} or {{steps.<step-id>.response.<path>}}.
func parseStepTemplateReference(rawValue string) (stepTemplateReference, bool, error) {
	emptyReference := stepTemplateReference{stepID: "", path: nil}
	trimmedValue := strings.TrimSpace(rawValue)
	if !strings.HasPrefix(trimmedValue, templatePrefix) || !strings.HasSuffix(trimmedValue, templateSuffix) {
		return emptyReference, false, nil
	}

	expression := strings.TrimSpace(trimmedValue[len(templatePrefix) : len(trimmedValue)-len(templateSuffix)])
	if !strings.HasPrefix(expression, templateStepsPrefix) {
		return emptyReference, false, nil
	}

	expressionWithoutPrefix := strings.TrimPrefix(expression, templateStepsPrefix)
	parts := strings.Split(expressionWithoutPrefix, ".")
	if len(parts) < templateMinParts {
		return emptyReference, false, fmt.Errorf(
			"unsupported request template %q: expected {{steps.<step-id>.response[.<path>]}}",
			rawValue,
		)
	}

	stepID := strings.TrimSpace(parts[0])
	if stepID == "" {
		return emptyReference, false, fmt.Errorf("unsupported request template %q: step id is empty", rawValue)
	}
	if parts[1] != templateResponseSegment {
		return emptyReference, false, fmt.Errorf(
			"unsupported request template %q: expected .response after step id",
			rawValue,
		)
	}

	path := make([]string, 0, max(len(parts)-templatePathOffset, 0))
	for _, pathSegment := range parts[templatePathOffset:] {
		normalizedSegment := strings.TrimSpace(pathSegment)
		if normalizedSegment == "" {
			return emptyReference, false, fmt.Errorf(
				"unsupported request template %q: path segment is empty",
				rawValue,
			)
		}
		path = append(path, normalizedSegment)
	}

	return stepTemplateReference{stepID: stepID, path: path}, true, nil
}

// resolveTemplateReference reads the value from the output steps by step id and optional path.
func resolveTemplateReference(reference stepTemplateReference, stepOutputs map[string]any) (any, error) {
	stepOutput, exists := stepOutputs[reference.stepID]
	if !exists {
		return nil, fmt.Errorf(
			"request template references step %q, but its output is not available",
			reference.stepID,
		)
	}

	currentValue := stepOutput
	for _, pathSegment := range reference.path {
		nextValue, err := resolvePathSegment(currentValue, pathSegment)
		if err != nil {
			return nil, fmt.Errorf(
				"request template step %q path %q: %w",
				reference.stepID,
				strings.Join(reference.path, "."),
				err,
			)
		}
		currentValue = nextValue
	}

	return currentValue, nil
}

// resolvePathSegment retrieves a child value from map/slice/struct one path segment at a time.
func resolvePathSegment(value any, segment string) (any, error) {
	reflectedValue, err := dereferenceValue(reflect.ValueOf(value))
	if err != nil {
		return nil, err
	}

	kind := reflectedValue.Kind()
	if kind == reflect.Map {
		if reflectedValue.Type().Key().Kind() != reflect.String {
			return nil, fmt.Errorf("map key type %s is not supported", reflectedValue.Type().Key())
		}

		mapValue := reflectedValue.MapIndex(reflect.ValueOf(segment))
		if !mapValue.IsValid() {
			return nil, fmt.Errorf("key %q is not found", segment)
		}

		return mapValue.Interface(), nil
	}
	if kind == reflect.Slice || kind == reflect.Array {
		index, parseErr := strconv.Atoi(segment)
		if parseErr != nil {
			return nil, fmt.Errorf("segment %q is not a valid index", segment)
		}
		if index < 0 || index >= reflectedValue.Len() {
			return nil, fmt.Errorf("index %d is out of range", index)
		}

		return reflectedValue.Index(index).Interface(), nil
	}
	if kind == reflect.Struct {
		fieldValue, exists := findStructFieldByPathSegment(reflectedValue, segment)
		if !exists {
			return nil, fmt.Errorf("field %q is not found", segment)
		}
		if !fieldValue.CanInterface() {
			return nil, fmt.Errorf("field %q is not exported", segment)
		}

		return fieldValue.Interface(), nil
	}

	return nil, fmt.Errorf("value type %T does not support path access", value)
}

// dereferenceValue unwraps the interface/pointer layers before accessing the path.
func dereferenceValue(value reflect.Value) (reflect.Value, error) {
	if !value.IsValid() {
		return reflect.Value{}, errors.New("value is invalid")
	}

	current := value
	for current.Kind() == reflect.Pointer || current.Kind() == reflect.Interface {
		if current.IsNil() {
			return reflect.Value{}, errors.New("value is nil")
		}
		current = current.Elem()
	}

	return current, nil
}

// findStructFieldByPathSegment resolves the field by the name of the Go field or the name from the json tag.
func findStructFieldByPathSegment(structValue reflect.Value, segment string) (reflect.Value, bool) {
	structType := structValue.Type()
	for fieldIndex := range structType.NumField() {
		structField := structType.Field(fieldIndex)
		if !structField.IsExported() {
			continue
		}

		if structField.Name == segment {
			return structValue.Field(fieldIndex), true
		}

		jsonTag := strings.Split(structField.Tag.Get("json"), ",")[0]
		if jsonTag == "" || jsonTag == "-" {
			continue
		}
		if jsonTag == segment {
			return structValue.Field(fieldIndex), true
		}
	}

	return reflect.Value{}, false
}
