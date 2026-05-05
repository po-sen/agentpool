package agent

import (
	"strings"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestBuildSystemPromptListsToolProtocol(t *testing.T) {
	prompt := buildSystemPrompt([]outbound.ToolDefinition{
		{Name: "echo", Description: "Returns text"},
		{
			Name:        "workspace",
			Description: "Lists workspace files and stats workspace paths without reading file contents.",
			Arguments: []outbound.ToolArgumentDefinition{
				{
					Name:        "operation",
					Description: `Operation to run. Supported values: "list" or "stat".`,
					Required:    true,
					Example:     "list",
				},
			},
		},
		{
			Name:        "sandbox_exec",
			Description: "Runs a shell command inside the prepared sandbox with /workspace/work as the working directory.",
			Arguments: []outbound.ToolArgumentDefinition{
				{
					Name:        "command",
					Description: "Shell command to run inside the sandbox. Read inputs from /workspace/input and write generated files under /workspace/work.",
					Required:    true,
					Example:     "sed -n '1,160p' /workspace/input/README.md",
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
		"For subjective discussion or simple conversation, answer directly when no tool is needed.",
		"When sandbox_exec is available, prefer using it before the final answer for deterministic or verifiable tasks",
		"computed, inspected, tested, counted, searched, or derived by running a command or script",
		"Do not guess exact answers when sandbox_exec can cheaply verify them.",
		"Use sandbox_exec for exact arithmetic, counts, hashes, encoding/decoding checks, file content inspection, grep/search, data sorting/filtering/transformation, tests, builds, linters, and code behavior checks.",
		"Only call tools listed under Available tools.",
		"Never invent tool names.",
		"Tool names are exact and case-sensitive.",
		"echo: Returns text",
		"Arguments: none",
		"workspace: Lists workspace files and stats workspace paths without reading file contents.",
		`operation (required): Operation to run. Supported values: "list" or "stat". Example: list`,
		"sandbox_exec: Runs a shell command inside the prepared sandbox with /workspace/work as the working directory.",
		"command (required): Shell command to run inside the sandbox. Read inputs from /workspace/input and write generated files under /workspace/work. Example: sed -n '1,160p' /workspace/input/README.md",
		"timeout_seconds (optional): Optional timeout in seconds. Must be a positive integer and no more than the configured maximum. Example: 10",
		"Use workspace with operation=list to discover files",
		"workspace does not read file contents.",
		"Uploaded file paths in the task message are relative to /workspace/input when using sandbox_exec.",
		"Original run inputs are under /workspace/input and are read-only.",
		"Generated scripts, temp files, tests, and outputs belong under /workspace/work.",
		"To read file contents, use sandbox_exec",
		"grep -R \"keyword\" /workspace/input",
		"For sandbox_exec, the current working directory is /workspace/work.",
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

	for _, oldToolName := range []string{"list_" + "files", "read_" + "file", "run_" + "shell"} {
		if strings.Contains(prompt, oldToolName) {
			t.Fatalf("prompt contains old tool name %q:\n%s", oldToolName, prompt)
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

func TestBuildSystemPromptDoesNotPreferSandboxExecWhenUnavailable(t *testing.T) {
	prompt := buildSystemPrompt([]outbound.ToolDefinition{
		{Name: "workspace", Description: "Lists workspace files and stats workspace paths."},
	})

	if strings.Contains(prompt, "prefer using it before the final answer") {
		t.Fatalf("prompt prefers sandbox_exec when it is unavailable:\n%s", prompt)
	}
	if strings.Contains(prompt, "Do not guess exact answers when sandbox_exec can cheaply verify them.") {
		t.Fatalf("prompt includes sandbox_exec verification rule when it is unavailable:\n%s", prompt)
	}
}
