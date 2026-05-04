package agent

import (
	"strings"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestBuildSystemPromptListsToolProtocol(t *testing.T) {
	prompt := buildSystemPrompt([]outbound.ToolDefinition{
		{Name: "echo", Description: "Returns text"},
		{Name: "list_files", Description: "Lists files"},
		{Name: "read_file", Description: "Reads text files"},
	})

	for _, want := range []string{
		"AgentPool is running a task.",
		`{"type":"final","summary":"..."}`,
		`{"type":"tool_call","tool":"<tool_name>","arguments":{"key":"value"}}`,
		"Call tools when they are useful",
		"Do not call tools when the task can be answered directly.",
		"echo: Returns text",
		"list_files: Lists files",
		"read_file: Reads text files",
		"discover uploaded files with list_files",
		"no markdown fences",
		"Do not return tool_result.",
		"Do not return multiple JSON objects.",
		"return another tool_call or a final answer",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt does not contain %q:\n%s", want, prompt)
		}
	}
}

func TestBuildSystemPromptHandlesNoTools(t *testing.T) {
	prompt := buildSystemPrompt(nil)
	if !strings.Contains(prompt, "- none") {
		t.Fatalf("prompt does not contain no-tools marker:\n%s", prompt)
	}
}
