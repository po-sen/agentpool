package agent

import (
	"fmt"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

const maxToolObservationLength = 8 << 10
const toolErrorRetryGuidance = "This may be partial or expected; if the task remains possible, try a different approach before final.\n"

func buildToolObservation(tool string, _ map[string]string, result outbound.ToolResult) string {
	content := result.Content
	if result.IsError {
		prefix := fmt.Sprintf("Tool error for %s:\n", tool)

		return prefix + toolErrorRetryGuidance + truncateToolObservationContent(prefix+toolErrorRetryGuidance, content)
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
