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
	actionParseValid actionParseStatus = iota
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
	raw, parseErr := decodeActionObject(trimmed)
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

func decodeActionObject(content string) (map[string]any, *actionParseError) {
	decoded, extraErr, decodeErr := decodeSingleJSONValue(content)
	if decodeErr != nil {
		return nil, newActionParseError(
			actionParseCodeInvalidJSON,
			invalidJSONActionMessage(decodeErr),
			invalidJSONActionHint(decodeErr),
			decodeErr,
		)
	}
	if !errors.Is(extraErr, io.EOF) {
		return nil, newActionParseError(
			actionParseCodeMultipleJSONValues,
			"model response must contain exactly one JSON object",
			`Return only one object, for example {"type":"final","summary":"..."}`,
			extraErr,
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

func decodeSingleJSONValue(content string) (any, error, error) {
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.UseNumber()

	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, nil, err
	}

	return decoded, decoder.Decode(&struct{}{}), nil
}

func invalidJSONActionMessage(err error) string {
	if isInvalidJSONStringEscapeError(err) {
		return `model response was not valid JSON: JSON strings cannot use backslash before arbitrary punctuation such as \*`
	}

	return "model response was not valid JSON"
}

func invalidJSONActionHint(err error) string {
	if isInvalidJSONStringEscapeError(err) {
		return `JSON strings only allow escapes like \", \\, \n, \t, or \uXXXX. If a shell command needs \*, encode it as \\* in JSON, for example {"type":"tool_call","tool":"sandbox_exec","arguments":{"command":"expr 123 \\* 654321 \\* 2"}}.`
	}

	return `Return exactly one JSON object like {"type":"final","summary":"..."}`
}

func isInvalidJSONStringEscapeError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "in string escape code")
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
