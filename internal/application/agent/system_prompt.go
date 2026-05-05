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
	builder.WriteString("For a tool call, use {\"type\":\"tool_call\",\"tool\":\"<tool_name>\",\"arguments\":{\"key\":\"value\"}}.\n")
	builder.WriteString("Call tools when they are useful to inspect available information. Do not call tools when the task can be answered directly.\n")
	builder.WriteString("Only call tools listed under Available tools. Never invent tool names. Tool names are exact and case-sensitive.\n")
	builder.WriteString("If Available tools is \"- none\", do not call any tool. Return {\"type\":\"final\",\"summary\":\"...\"} directly.\n")
	builder.WriteString("If the user asks you to use a tool that is not available, explain that the tool is unavailable and answer directly if possible.\n")
	builder.WriteString("When list_files and read_file are available, discover uploaded files with list_files before reading only the files needed for the task.\n")
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
