package run

import (
	"strings"
	"time"
)

const (
	// MaxAgentTurnPreviewLength bounds stored model response previews exposed through read APIs.
	MaxAgentTurnPreviewLength = 4 << 10

	// AgentTurnStatusModelResponse identifies a generic model response turn.
	AgentTurnStatusModelResponse = "model_response"
	// AgentTurnStatusNaturalLanguageFinal identifies a natural-language final answer.
	AgentTurnStatusNaturalLanguageFinal = "natural_language_final"
	// AgentTurnStatusFinal identifies a parsed final action.
	AgentTurnStatusFinal = "final"
	// AgentTurnStatusToolCall identifies a parsed tool call action.
	AgentTurnStatusToolCall = "tool_call"
	// AgentTurnStatusProtocolError identifies a model response protocol error.
	AgentTurnStatusProtocolError = "protocol_error"
	// AgentTurnStatusInvalidToolCall identifies a tool call for a non-advertised tool.
	AgentTurnStatusInvalidToolCall = "invalid_tool_call"
	// AgentTurnStatusModelError identifies a model generation or model-output error.
	AgentTurnStatusModelError = "model_error"
	// AgentTurnStatusToolError identifies a tool execution error following a tool-call turn.
	AgentTurnStatusToolError = "tool_error"
	// AgentTurnStatusMaxTurns identifies the agent loop max-turns diagnostic.
	AgentTurnStatusMaxTurns = "max_turns"

	// AgentTurnActionTypeFinal identifies a final action.
	AgentTurnActionTypeFinal = "final"
	// AgentTurnActionTypeToolCall identifies a tool_call action.
	AgentTurnActionTypeToolCall = "tool_call"
)

// AgentTurn records one provider-neutral model-loop diagnostic observed during a run.
type AgentTurn struct {
	Index           int
	Status          string
	ActionType      string
	ToolName        string
	Message         string
	ResponsePreview string
	StartedAt       time.Time
	EndedAt         time.Time
}

func copyAgentTurns(turns []AgentTurn) []AgentTurn {
	if len(turns) == 0 {
		return nil
	}

	copied := make([]AgentTurn, 0, len(turns))
	for _, turn := range turns {
		if turn.Index <= 0 {
			continue
		}

		item := turn
		item.Status = strings.TrimSpace(turn.Status)
		item.ActionType = strings.TrimSpace(turn.ActionType)
		item.ToolName = strings.TrimSpace(turn.ToolName)
		item.Message = strings.TrimSpace(turn.Message)
		item.ResponsePreview = truncateUTF8Text(turn.ResponsePreview, MaxAgentTurnPreviewLength)
		copied = append(copied, item)
	}

	return copied
}
