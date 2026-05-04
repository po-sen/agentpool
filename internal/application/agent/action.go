package agent

import "encoding/json"

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

func parseAction(content string) (action, bool) {
	var parsed action
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return action{}, false
	}

	switch parsed.Type {
	case actionTypeFinal, actionTypeToolCall:
		if parsed.Arguments == nil {
			parsed.Arguments = map[string]string{}
		}

		return parsed, true
	default:
		return action{}, false
	}
}
