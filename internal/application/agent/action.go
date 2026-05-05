package agent

import (
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strconv"
	"strings"
)

type actionType string

const (
	actionTypeFinal    actionType = "final"
	actionTypeToolCall actionType = "tool_call"
)

const (
	actionParseCodeInvalidJSON          = "invalid_json"
	actionParseCodeMultipleJSONValues   = "multiple_json_values"
	actionParseCodeNonObjectAction      = "non_object_action"
	actionParseCodeUnknownField         = "unknown_field"
	actionParseCodeMissingType          = "missing_type"
	actionParseCodeInvalidTypeField     = "invalid_type_field"
	actionParseCodeUnknownActionType    = "unknown_action_type"
	actionParseCodeMissingSummary       = "missing_summary"
	actionParseCodeInvalidSummary       = "invalid_summary"
	actionParseCodeMissingTool          = "missing_tool"
	actionParseCodeInvalidTool          = "invalid_tool"
	actionParseCodeInvalidArguments     = "invalid_arguments"
	actionParseCodeInvalidArgumentValue = "invalid_argument_value"
)

const (
	finalActionHint    = `Return {"type":"final","summary":"..."}`
	protocolActionHint = `Return {"type":"final","summary":"..."} or {"type":"tool_call","tool":"workspace","arguments":{"operation":"list","area":"all","path":"."}}`
	toolCallActionHint = `Return {"type":"tool_call","tool":"workspace","arguments":{"operation":"list","area":"all","path":"."}}`
)

type action struct {
	Type      actionType
	Summary   string
	Tool      string
	Arguments map[string]string
}

type actionParseStatus int

const (
	actionParseNaturalLanguage actionParseStatus = iota
	actionParseValid
	actionParseProtocolError
)

type actionParseError struct {
	Code    string
	Message string
	Hint    string
	cause   error
}

func (e actionParseError) Error() string {
	return e.Message
}

func (e actionParseError) Unwrap() error {
	return e.cause
}

type actionParseResult struct {
	action   action
	status   actionParseStatus
	parseErr actionParseError
	err      error
}

func parseAction(content string) actionParseResult {
	trimmed := strings.TrimSpace(content)
	if !looksLikeProtocolResponse(trimmed) {
		embedded, ok, parseErr := extractSingleEmbeddedJSONObject(trimmed)
		if parseErr != nil {
			return protocolError(*parseErr)
		}
		if !ok {
			return actionParseResult{status: actionParseNaturalLanguage}
		}
		trimmed = embedded
	}

	normalized := normalizeProtocolResponse(trimmed)
	raw, parseErr := decodeActionObject(normalized)
	if parseErr != nil {
		return protocolError(*parseErr)
	}

	parsed, parseErr := parseActionObject(raw)
	if parseErr != nil {
		return protocolError(*parseErr)
	}

	return actionParseResult{action: parsed, status: actionParseValid}
}

func protocolError(parseErr actionParseError) actionParseResult {
	return actionParseResult{status: actionParseProtocolError, parseErr: parseErr, err: parseErr}
}

func normalizeProtocolResponse(content string) string {
	if !strings.HasPrefix(content, "```") {
		return content
	}

	rest := strings.TrimPrefix(content, "```")
	lineEnd := strings.IndexAny(rest, "\r\n")
	if lineEnd < 0 {
		return content
	}

	label := strings.TrimSpace(rest[:lineEnd])
	if label != "" && !strings.EqualFold(label, "json") {
		return content
	}

	innerWithFence := strings.TrimSpace(rest[lineEnd:])
	if !strings.HasSuffix(innerWithFence, "```") {
		return content
	}

	inner := strings.TrimSpace(strings.TrimSuffix(innerWithFence, "```"))
	if inner == "" {
		return content
	}

	return inner
}

func decodeActionObject(content string) (map[string]any, *actionParseError) {
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.UseNumber()

	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, newActionParseError(
			actionParseCodeInvalidJSON,
			"model response was not valid JSON",
			`Return exactly one JSON object like {"type":"final","summary":"..."}`,
			err,
		)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, newActionParseError(
			actionParseCodeMultipleJSONValues,
			"model response must contain exactly one JSON object",
			`Return only one object, for example {"type":"final","summary":"..."}`,
			err,
		)
	}

	raw, ok := decoded.(map[string]any)
	if !ok {
		return nil, newActionParseError(
			actionParseCodeNonObjectAction,
			"model response must be a JSON object",
			protocolActionHint,
			nil,
		)
	}

	return raw, nil
}

func parseActionObject(raw map[string]any) (action, *actionParseError) {
	actionTypeValue, parseErr := parseActionType(raw)
	if parseErr != nil {
		return action{}, parseErr
	}

	switch actionTypeValue {
	case actionTypeFinal:
		return parseFinalAction(raw)
	case actionTypeToolCall:
		return parseToolCallAction(raw)
	default:
		return action{}, newActionParseError(
			actionParseCodeUnknownActionType,
			"action type must be final or tool_call",
			protocolActionHint,
			nil,
		)
	}
}

func parseActionType(raw map[string]any) (actionType, *actionParseError) {
	value, ok := raw["type"]
	if !ok {
		return "", newActionParseError(
			actionParseCodeMissingType,
			"action.type is required",
			protocolActionHint,
			nil,
		)
	}

	typeValue, ok := value.(string)
	if !ok {
		return "", newActionParseError(
			actionParseCodeInvalidTypeField,
			"action.type must be a string",
			`Use "final" or "tool_call" as the type value.`,
			nil,
		)
	}

	return actionType(typeValue), nil
}

func parseFinalAction(raw map[string]any) (action, *actionParseError) {
	if parseErr := rejectUnknownFields(raw, map[string]struct{}{"type": {}, "summary": {}}); parseErr != nil {
		return action{}, parseErr
	}

	value, ok := raw["summary"]
	if !ok {
		return action{}, newActionParseError(
			actionParseCodeMissingSummary,
			"final.summary is required",
			finalActionHint,
			nil,
		)
	}

	summary, ok := scalarToString(value)
	if !ok {
		return action{}, newActionParseError(
			actionParseCodeInvalidSummary,
			"final.summary must be a string, boolean, or number",
			finalActionHint,
			nil,
		)
	}
	if strings.TrimSpace(summary) == "" {
		return action{}, newActionParseError(
			actionParseCodeInvalidSummary,
			"final.summary must not be empty",
			finalActionHint,
			nil,
		)
	}

	return action{Type: actionTypeFinal, Summary: summary}, nil
}

func parseToolCallAction(raw map[string]any) (action, *actionParseError) {
	if parseErr := rejectUnknownFields(raw, map[string]struct{}{"type": {}, "tool": {}, "arguments": {}}); parseErr != nil {
		return action{}, parseErr
	}

	toolValue, ok := raw["tool"]
	if !ok {
		return action{}, newActionParseError(
			actionParseCodeMissingTool,
			"tool_call.tool is required",
			toolCallActionHint,
			nil,
		)
	}
	tool, ok := toolValue.(string)
	if !ok {
		return action{}, newActionParseError(
			actionParseCodeInvalidTool,
			"tool_call.tool must be a string",
			toolCallActionHint,
			nil,
		)
	}
	if strings.TrimSpace(tool) == "" {
		return action{}, newActionParseError(
			actionParseCodeInvalidTool,
			"tool_call.tool must not be empty",
			toolCallActionHint,
			nil,
		)
	}

	arguments, parseErr := parseToolArguments(raw)
	if parseErr != nil {
		return action{}, parseErr
	}

	return action{Type: actionTypeToolCall, Tool: tool, Arguments: arguments}, nil
}

func parseToolArguments(raw map[string]any) (map[string]string, *actionParseError) {
	value, ok := raw["arguments"]
	if !ok {
		return map[string]string{}, nil
	}

	rawArguments, ok := value.(map[string]any)
	if !ok {
		return nil, newActionParseError(
			actionParseCodeInvalidArguments,
			"tool_call.arguments must be an object",
			toolCallActionHint,
			nil,
		)
	}

	arguments := make(map[string]string, len(rawArguments))
	for key, argumentValue := range rawArguments {
		value, ok := scalarToString(argumentValue)
		if !ok {
			return nil, newActionParseError(
				actionParseCodeInvalidArgumentValue,
				"tool_call.arguments values must be strings, booleans, or numbers",
				`Use flat string-compatible arguments, for example {"path":"README.md"}.`,
				nil,
			)
		}
		arguments[key] = value
	}

	return arguments, nil
}

func rejectUnknownFields(raw map[string]any, allowed map[string]struct{}) *actionParseError {
	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		if _, ok := allowed[key]; !ok {
			return newActionParseError(
				actionParseCodeUnknownField,
				"unsupported action field "+strconv.Quote(key),
				"Return exactly one JSON object with only the allowed fields.",
				nil,
			)
		}
	}

	return nil
}

func scalarToString(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	case bool:
		return strconv.FormatBool(typed), true
	case json.Number:
		return typed.String(), true
	default:
		return "", false
	}
}

func newActionParseError(code string, message string, hint string, cause error) *actionParseError {
	return &actionParseError{Code: code, Message: message, Hint: hint, cause: cause}
}

func looksLikeProtocolResponse(content string) bool {
	return strings.HasPrefix(content, "{") ||
		strings.HasPrefix(content, "[") ||
		strings.HasPrefix(content, "```")
}

func extractSingleEmbeddedJSONObject(content string) (string, bool, *actionParseError) {
	var found string
	count := 0
	for offset := 0; offset < len(content); {
		relativeStart := strings.Index(content[offset:], "{")
		if relativeStart < 0 {
			break
		}
		start := offset + relativeStart
		candidate, consumed, ok := decodeJSONObjectPrefix(content[start:])
		if !ok {
			offset = start + 1
			continue
		}
		count++
		if count > 1 {
			return "", false, newActionParseError(
				actionParseCodeMultipleJSONValues,
				"model response must contain exactly one JSON object",
				`Return only one object, for example {"type":"final","summary":"..."}`,
				nil,
			)
		}
		found = candidate
		offset = start + consumed
	}
	if count == 0 {
		return "", false, nil
	}

	return found, true, nil
}

func decodeJSONObjectPrefix(content string) (string, int, bool) {
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.UseNumber()

	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return "", 0, false
	}
	if _, ok := decoded.(map[string]any); !ok {
		return "", 0, false
	}
	consumed := int(decoder.InputOffset())

	return content[:consumed], consumed, true
}
