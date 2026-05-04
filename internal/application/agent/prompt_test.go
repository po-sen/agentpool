package agent

import (
	"strings"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestBuildSystemPromptListsToolProtocol(t *testing.T) {
	prompt := buildSystemPrompt([]outbound.ToolDefinition{
		{Name: "echo", Description: "Returns text"},
	})

	for _, want := range []string{
		"AgentPool is running a task.",
		`{"type":"final","summary":"..."}`,
		`{"type":"tool_call","tool":"echo","arguments":{"text":"..."}}`,
		"echo: Returns text",
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
