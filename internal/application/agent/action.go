package agent

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
)

type actionType string

const (
	actionTypeFinal    actionType = "final"
	actionTypeToolCall actionType = "tool_call"
)

type action struct {
	Type      actionType        `json:"type"`
	Summary   string            `json:"summary,omitempty"`
	Tool      string            `json:"tool,omitempty"`
	Arguments map[string]string `json:"arguments,omitempty"`
}

type actionParseStatus int

const (
	actionParseNaturalLanguage actionParseStatus = iota
	actionParseValid
	actionParseProtocolError
)

type actionParseResult struct {
	action action
	status actionParseStatus
	err    error
}

func parseAction(content string) actionParseResult {
	trimmed := strings.TrimSpace(content)
	if !looksLikeJSON(trimmed) {
		return actionParseResult{status: actionParseNaturalLanguage}
	}

	var parsed action
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&parsed); err != nil {
		return actionParseResult{status: actionParseProtocolError, err: err}
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return actionParseResult{status: actionParseProtocolError, err: errors.New("response must contain exactly one JSON value")}
	}

	switch parsed.Type {
	case actionTypeFinal:
		if strings.TrimSpace(parsed.Summary) == "" {
			return actionParseResult{status: actionParseProtocolError, err: errors.New("final summary is required")}
		}

		return actionParseResult{action: parsed, status: actionParseValid}
	case actionTypeToolCall:
		if strings.TrimSpace(parsed.Tool) == "" {
			return actionParseResult{status: actionParseProtocolError, err: errors.New("tool call tool is required")}
		}
		if parsed.Arguments == nil {
			parsed.Arguments = map[string]string{}
		}

		return actionParseResult{action: parsed, status: actionParseValid}
	default:
		return actionParseResult{status: actionParseProtocolError, err: errors.New("unknown or missing action type")}
	}
}

func looksLikeJSON(content string) bool {
	return strings.HasPrefix(content, "{") || strings.HasPrefix(content, "[")
}
