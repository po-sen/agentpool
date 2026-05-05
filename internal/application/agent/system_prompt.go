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
	builder.WriteString("Call tools when they are useful to inspect available information. For subjective discussion or simple conversation, answer directly when no tool is needed.\n")
	if toolIsDefined(tools, "sandbox_exec") {
		builder.WriteString("When sandbox_exec is available, prefer using it before the final answer for deterministic or verifiable tasks that can be computed, inspected, tested, counted, searched, or derived by running a command or script. Do not guess exact answers when sandbox_exec can cheaply verify them.\n")
		builder.WriteString("Use sandbox_exec for exact arithmetic, counts, hashes, encoding/decoding checks, file content inspection, grep/search, data sorting/filtering/transformation, tests, builds, linters, and code behavior checks.\n")
	}
	builder.WriteString("Only call tools listed under Available tools. Never invent tool names. Tool names are exact and case-sensitive.\n")
	builder.WriteString("If Available tools is \"- none\", do not call any tool. Return {\"type\":\"final\",\"summary\":\"...\"} directly.\n")
	builder.WriteString("If the user asks you to use a tool that is not available, explain that the tool is unavailable and answer directly if possible.\n")
	builder.WriteString("Use workspace with operation=list to discover files and operation=stat to inspect metadata. workspace does not read file contents.\n")
	builder.WriteString("Uploaded file paths in the task message are relative to /workspace/input when using sandbox_exec.\n")
	builder.WriteString("Original run inputs are under /workspace/input and are read-only. Generated scripts, temp files, tests, and outputs belong under /workspace/work.\n")
	builder.WriteString("To read file contents, use sandbox_exec with concrete commands such as sed -n '1,160p' /workspace/input/README.md or cat /workspace/input/README.md.\n")
	builder.WriteString("To search files, use sandbox_exec with concrete commands such as grep -R \"keyword\" /workspace/input.\n")
	builder.WriteString("For sandbox_exec, the current working directory is /workspace/work. Do not modify /workspace/input.\n")
	builder.WriteString("Never use placeholder argument values such as <file_path>; discover or use the actual path before calling a tool.\n")
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
