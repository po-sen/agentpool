package agent

import (
	"fmt"
	"strings"
)

type unavailableToolCorrectionRequest struct {
	RequestedTool  string
	AvailableTools []string
}

func buildProtocolCorrectionMessage(parseErr actionParseError) string {
	message := parseErr.Message
	if message == "" {
		message = "model response did not match AgentPool action protocol"
	}
	hint := parseErr.Hint
	if hint == "" {
		hint = `Return {"type":"final","summary":"..."} or {"type":"tool_call","tool":"read_file","arguments":{"path":"README.md"}}.`
	}

	return `Protocol error:
Your previous response was invalid because ` + message + `.
` + hint + `
Return exactly one JSON object with only the allowed fields.
Examples:
{"type":"final","summary":"Finished the task."}
{"type":"tool_call","tool":"read_file","arguments":{"path":"README.md"}}
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
