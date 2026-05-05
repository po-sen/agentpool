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
		{
			Name:        "read_file",
			Description: "Reads text files",
			Arguments: []outbound.ToolArgumentDefinition{
				{
					Name:        "path",
					Description: "Relative path of the uploaded workspace file to read.",
					Required:    true,
					Example:     "README.md",
				},
			},
		},
		{
			Name:        "run_shell",
			Description: "Runs a command inside the prepared sandbox workspace.",
			Arguments: []outbound.ToolArgumentDefinition{
				{
					Name:        "command",
					Description: "Shell command to run inside the prepared sandbox workspace. Uploaded files are available as relative paths from the workspace root.",
					Required:    true,
					Example:     "wc -m README.md",
				},
				{
					Name:        "timeout_seconds",
					Description: "Optional timeout in seconds. Must be a positive integer and no more than the configured maximum.",
					Required:    false,
					Example:     "10",
				},
			},
		},
	})

	for _, want := range []string{
		"AgentPool is running a task.",
		`{"type":"final","summary":"..."}`,
		`{"type":"tool_call","tool":"<tool_name>","arguments":{"key":"value"}}`,
		"Call tools when they are useful",
		"Do not call tools when the task can be answered directly.",
		"Only call tools listed under Available tools.",
		"Never invent tool names.",
		"Tool names are exact and case-sensitive.",
		"echo: Returns text",
		"list_files: Lists files",
		"Arguments: none",
		"read_file: Reads text files",
		"path (required): Relative path of the uploaded workspace file to read. Example: README.md",
		"run_shell: Runs a command inside the prepared sandbox workspace.",
		"command (required): Shell command to run inside the prepared sandbox workspace. Uploaded files are available as relative paths from the workspace root. Example: wc -m README.md",
		"timeout_seconds (optional): Optional timeout in seconds. Must be a positive integer and no more than the configured maximum. Example: 10",
		"discover uploaded files with list_files",
		"Uploaded files are available as relative paths in the workspace root.",
		"If exactly one uploaded file is listed",
		"For run_shell, the current working directory is the workspace root.",
		"wc -m README.md",
		"Never use placeholder argument values such as <file_path>",
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
	if !strings.Contains(prompt, "Available tools:\n- none") {
		t.Fatalf("prompt does not contain no-tools marker:\n%s", prompt)
	}
	if !strings.Contains(prompt, "do not call any tool") {
		t.Fatalf("prompt does not forbid tools when none are available:\n%s", prompt)
	}
	if !strings.Contains(prompt, `Return {"type":"final","summary":"..."} directly`) {
		t.Fatalf("prompt does not require direct final response:\n%s", prompt)
	}
	if strings.Contains(prompt, "Arguments:") {
		t.Fatalf("no-tools prompt contains arguments metadata:\n%s", prompt)
	}
}
