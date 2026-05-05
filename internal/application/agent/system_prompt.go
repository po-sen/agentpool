package agent

import (
	"strings"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func buildSystemPrompt(tools []outbound.ToolDefinition) string {
	var builder strings.Builder
	builder.WriteString("AgentPool is running one task.\n\n")
	builder.WriteString("Output protocol:\n")
	builder.WriteString("- Return exactly one JSON object, no markdown fences.\n")
	builder.WriteString("- Final: {\"type\":\"final\",\"summary\":\"...\"}\n")
	builder.WriteString("- Tool call: {\"type\":\"tool_call\",\"tool\":\"<tool_name>\",\"arguments\":{\"key\":\"value\"}}\n")
	builder.WriteString("- Never return tool_result or multiple JSON objects.\n")
	builder.WriteString("- Only call tools listed under Available tools. Never invent tool names.\n\n")
	builder.WriteString("Tool policy:\n")
	if toolIsDefined(tools, "sandbox_exec") {
		builder.WriteString("- If sandbox_exec is available and the task has an exact or verifiable answer, call sandbox_exec before final. Do not guess exact answers when sandbox_exec can verify them.\n")
		builder.WriteString("- Use sandbox_exec for arithmetic, counts, searches, file content inspection, data transforms, tests, builds, linters, and code behavior checks.\n")
	}
	builder.WriteString("- For subjective discussion, architecture advice, brainstorming, or simple conversation, answer directly when no command is needed.\n")
	builder.WriteString("- If a needed tool is unavailable, answer with what can be known and say what could not be verified.\n\n")
	builder.WriteString("Workspace:\n")
	builder.WriteString("- /workspace/input contains read-only run inputs.\n")
	builder.WriteString("- /workspace/work is writable and is the sandbox_exec working directory.\n")
	builder.WriteString("- Uploaded file paths in the task are relative to /workspace/input.\n")
	builder.WriteString("- Use workspace to discover paths first when files are involved.\n")
	builder.WriteString("- workspace accepts either area plus relative path, or full /workspace/input|work virtual paths.\n")
	builder.WriteString("- Use /workspace/input for read-only inputs and /workspace/work for generated files.\n")
	builder.WriteString("- workspace only lists/stats paths; it does not read file contents.\n")
	builder.WriteString("- Use sandbox_exec to read/search/process file contents.\n")
	builder.WriteString("- Do not modify /workspace/input.\n")
	builder.WriteString("- Do not use placeholder paths like <file_path>.\n\n")
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
		writeToolArguments(&builder, tool.Arguments)
	}

	return builder.String()
}

func toolIsDefined(tools []outbound.ToolDefinition, name string) bool {
	for _, tool := range tools {
		if tool.Name == name {
			return true
		}
	}

	return false
}

func writeToolArguments(builder *strings.Builder, arguments []outbound.ToolArgumentDefinition) {
	if len(arguments) == 0 {
		builder.WriteString("  Arguments: none\n")

		return
	}

	builder.WriteString("  Arguments:\n")
	for _, argument := range arguments {
		builder.WriteString("  - ")
		builder.WriteString(argument.Name)
		builder.WriteString(" (")
		builder.WriteString(argumentRequirement(argument.Required))
		builder.WriteString("): ")
		builder.WriteString(argument.Description)
		if argument.Example != "" {
			builder.WriteString(" Example: ")
			builder.WriteString(argument.Example)
		}
		builder.WriteString("\n")
	}
}

func argumentRequirement(required bool) string {
	if required {
		return "required"
	}

	return "optional"
}
