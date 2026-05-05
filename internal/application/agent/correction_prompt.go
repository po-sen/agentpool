package agent

import (
	"fmt"
	"strings"
)

type unavailableToolCorrectionRequest struct {
	RequestedTool  string
	AvailableTools []string
}

type placeholderToolArgumentCorrectionRequest struct {
	Placeholders    []string
	AvailableTools  []string
	UploadedFileIDs []string
}

func buildProtocolCorrectionMessage(parseErr actionParseError) string {
	message := parseErr.Message
	if message == "" {
		message = "model response did not match AgentPool action protocol"
	}
	hint := parseErr.Hint
	if hint == "" {
		hint = `Return {"type":"final","summary":"..."} or {"type":"tool_call","tool":"workspace","arguments":{"operation":"list","area":"all","path":"."}}.`
	}

	return `Protocol error:
Your previous response was invalid because ` + message + `.
` + hint + `
Return exactly one JSON object with only the allowed fields.
Examples:
{"type":"final","summary":"Finished the task."}
{"type":"tool_call","tool":"workspace","arguments":{"operation":"list","area":"all","path":"."}}
Do not return tool_result. Do not return multiple JSON objects. Do not use markdown fences.`
}

func buildUnavailableToolCorrectionMessage(request unavailableToolCorrectionRequest) string {
	availableText := "none"
	if len(request.AvailableTools) > 0 {
		availableText = strings.Join(request.AvailableTools, ", ")
	}

	return fmt.Sprintf(`Tool call error:
The tool %q is not available.
Available tools: %s

You may only call tools listed in Available tools.
Do not invent tool names.
Tool names are exact and case-sensitive.
If no tools are available, return a final answer directly.`, request.RequestedTool, availableText)
}

func buildPlaceholderToolArgumentCorrectionMessage(request placeholderToolArgumentCorrectionRequest) string {
	availableText := "none"
	if len(request.AvailableTools) > 0 {
		availableText = strings.Join(request.AvailableTools, ", ")
	}
	placeholderText := "one or more arguments"
	if len(request.Placeholders) > 0 {
		placeholderText = strings.Join(request.Placeholders, ", ")
	}

	var builder strings.Builder
	builder.WriteString("Tool call error:\n")
	builder.WriteString("The previous tool call used placeholder argument values: ")
	builder.WriteString(placeholderText)
	builder.WriteString(".\n")
	builder.WriteString("Replace placeholders with concrete values before calling a tool. Do not use angle-bracket placeholders such as <file_path>.\n")
	if len(request.UploadedFileIDs) > 0 {
		builder.WriteString("Uploaded files: ")
		builder.WriteString(strings.Join(request.UploadedFileIDs, ", "))
		builder.WriteString(". Use /workspace/input paths when reading uploaded file contents through sandbox_exec.\n")
	} else {
		builder.WriteString("If a file path is needed, discover it with workspace operation=list before using sandbox_exec.\n")
	}
	builder.WriteString("Available tools: ")
	builder.WriteString(availableText)
	builder.WriteString("\nReturn exactly one JSON object.")

	return builder.String()
}
