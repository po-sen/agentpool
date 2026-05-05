package agent

import (
	"strings"
	"testing"
)

func TestBuildProtocolCorrectionMessageUsesParseErrorDetails(t *testing.T) {
	message := buildProtocolCorrectionMessage(actionParseError{
		Message: "final.summary must be a string, boolean, or number",
		Hint:    `Return {"type":"final","summary":"..."}`,
	})

	for _, want := range []string{
		"Protocol error:",
		"final.summary must be a string, boolean, or number",
		`Return {"type":"final","summary":"..."}`,
		`{"type":"final","summary":"Finished the task."}`,
		`{"type":"tool_call","tool":"read_file","arguments":{"path":"README.md"}}`,
		"Do not return tool_result.",
		"Do not return multiple JSON objects.",
		"Do not use markdown fences.",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("message does not contain %q:\n%s", want, message)
		}
	}
}

func TestBuildProtocolCorrectionMessageUsesFallbacks(t *testing.T) {
	message := buildProtocolCorrectionMessage(actionParseError{})

	if !strings.Contains(message, "model response did not match AgentPool action protocol") {
		t.Fatalf("message does not contain fallback parse error:\n%s", message)
	}
	if !strings.Contains(message, `Return {"type":"final","summary":"..."} or {"type":"tool_call","tool":"read_file","arguments":{"path":"README.md"}}.`) {
		t.Fatalf("message does not contain fallback hint:\n%s", message)
	}
}

func TestBuildUnavailableToolCorrectionMessageHandlesNoAvailableTools(t *testing.T) {
	message := buildUnavailableToolCorrectionMessage(unavailableToolCorrectionRequest{
		RequestedTool: "sh_script",
	})

	for _, want := range []string{
		`The tool "sh_script" is not available.`,
		"Available tools: none",
		"Do not invent tool names.",
		"Tool names are exact and case-sensitive.",
		"If no tools are available, return a final answer directly.",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("message does not contain %q:\n%s", want, message)
		}
	}
}

func TestBuildUnavailableToolCorrectionMessageListsAvailableTools(t *testing.T) {
	message := buildUnavailableToolCorrectionMessage(unavailableToolCorrectionRequest{
		RequestedTool:  "sh_script",
		AvailableTools: []string{"list_files", "read_file", "run_shell"},
	})

	if !strings.Contains(message, "Available tools: list_files, read_file, run_shell") {
		t.Fatalf("message does not list available tools:\n%s", message)
	}
	if !strings.Contains(message, `The tool "sh_script" is not available.`) {
		t.Fatalf("message does not mention requested tool:\n%s", message)
	}
}
