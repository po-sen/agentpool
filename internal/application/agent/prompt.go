package agent

import (
	"strings"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func buildSystemPrompt(tools []outbound.ToolDefinition) string {
	var builder strings.Builder
	builder.WriteString("AgentPool is running a task.\n")
	builder.WriteString("Respond with exactly one JSON object and no markdown fences.\n")
	builder.WriteString("For a final answer, use {\"type\":\"final\",\"summary\":\"...\"}.\n")
	builder.WriteString("For a tool call, use {\"type\":\"tool_call\",\"tool\":\"echo\",\"arguments\":{\"text\":\"...\"}}.\n")
	builder.WriteString("Do not return tool_result. Do not return multiple JSON objects.\n")
	builder.WriteString("After receiving a tool result, return another tool_call or a final answer.\n")
	builder.WriteString("Available tools:\n")

	if len(tools) == 0 {
		builder.WriteString("- none\n")

		return builder.String()
	}

	for _, tool := range tools {
		builder.WriteString("- ")
		builder.WriteString(tool.Name)
		builder.WriteString(": ")
		builder.WriteString(tool.Description)
		builder.WriteString("\n")
	}

	return builder.String()
}
