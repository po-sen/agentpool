package agent

import (
	"fmt"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

const maxToolObservationLength = 8 << 10
const toolErrorRetryGuidance = "This failed attempt is an observation, not a final answer. Do not repeat the same tool call; keep iterating with changed arguments or a different method until a tool call succeeds.\n"

func buildToolObservation(tool string, arguments map[string]string, result outbound.ToolResult) string {
	content := result.Content
	if result.IsError {
		prefix := fmt.Sprintf("Tool error for %s:\n", tool)
		strategyDirective := buildToolErrorStrategyDirective(tool, arguments, result)
		if strategyDirective != "" {
			strategyDirective += "\n"
		}

		return prefix + toolErrorRetryGuidance + strategyDirective + truncateToolObservationContent(prefix+toolErrorRetryGuidance+strategyDirective, content)
	}

	prefix := fmt.Sprintf("Tool result for %s:\n", tool)

	return prefix + truncateToolObservationContent(prefix, content)
}

func truncateToolObservationContent(prefix string, content string) string {
	budget := maxToolObservationLength - len(prefix)
	if budget <= 0 {
		return ""
	}

	return truncateAgentTurnText(content, budget)
}
