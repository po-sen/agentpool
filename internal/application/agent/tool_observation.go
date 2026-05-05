package agent

import (
	"fmt"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func buildToolObservation(tool string, result outbound.ToolResult) string {
	if result.IsError {
		return fmt.Sprintf("Tool error for %s:\n%s", tool, result.Content)
	}

	return fmt.Sprintf("Tool result for %s:\n%s", tool, result.Content)
}
