package agent

import (
	"strings"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestBuildSystemPromptListsToolProtocol(t *testing.T) {
	prompt := buildSystemPrompt(testPromptTools())

	for _, want := range []string{
		"AgentPool is running one task.",
		"Output protocol:",
		"Return exactly one JSON object, no markdown fences.",
		`Final: {"type":"final","summary":"..."}`,
		"summary is the complete user-facing answer, not a completion note.",
		`Tool call: {"type":"tool_call","tool":"<tool_name>","arguments":{"key":"value"}}`,
		"Preserve the user's requested language in final.summary.",
		"Never return tool_result or multiple JSON objects.",
		"Only call tools listed under Available tools.",
		"Never invent tool names.",
		"Available tools:",
		"echo: Returns text",
		"Arguments: none",
		"workspace: Lists or stats workspace paths without reading file contents.",
		`operation (required): Operation to run. Supported values: "list" or "stat". Example: list`,
		"sandbox_exec: Runs a command inside the sandbox from /workspace/work.",
		"command (required): Command to run inside the sandbox. Example: wc -l /workspace/input/README.md",
		"timeout_seconds (optional): Optional timeout in seconds. Must be a positive integer and no more than the configured maximum. Example: 10",
	} {
		assertPromptContains(t, prompt, want)
	}

	assertNoOldToolNames(t, prompt)
}

func TestBuildSystemPromptListsPriorityToolPolicy(t *testing.T) {
	prompt := buildSystemPrompt(testPromptTools())

	for _, want := range []string{
		"Tool policy:",
		"If sandbox_exec is available and the task has an exact or verifiable answer, call sandbox_exec before final.",
		"Do not guess exact answers when sandbox_exec can verify them.",
		"Use sandbox_exec for arithmetic, counts, searches, file content inspection, data transforms, tests, builds, linters, and code behavior checks.",
		"For subjective discussion, architecture advice, brainstorming, or simple conversation, return a final JSON action directly when no command is needed.",
		"If a needed tool is unavailable, answer with what can be known and say what could not be verified.",
	} {
		assertPromptContains(t, prompt, want)
	}
}

func TestBuildSystemPromptListsWorkspaceRules(t *testing.T) {
	prompt := buildSystemPrompt(testPromptTools())

	for _, want := range []string{
		"Workspace:",
		"/workspace/input contains read-only run inputs.",
		"/workspace/work is writable and is the sandbox_exec working directory.",
		"Uploaded file paths in the task are relative to /workspace/input.",
		"Use workspace to discover paths first when files are involved.",
		"workspace accepts either area plus relative path, or full /workspace/input|work virtual paths.",
		"Use /workspace/input for read-only inputs and /workspace/work for generated files.",
		"workspace only lists/stats paths; it does not read file contents.",
		"Use sandbox_exec to read/search/process file contents.",
		"Do not modify /workspace/input.",
		"Do not use placeholder paths like <file_path>.",
	} {
		assertPromptContains(t, prompt, want)
	}
}

func TestBuildSystemPromptHandlesNoTools(t *testing.T) {
	prompt := buildSystemPrompt(nil)

	assertPromptContains(t, prompt, "Available tools:\n- none")
	assertPromptContains(t, prompt, "If a needed tool is unavailable, answer with what can be known and say what could not be verified.")
	if strings.Contains(prompt, "Arguments:") {
		t.Fatalf("no-tools prompt contains arguments metadata:\n%s", prompt)
	}
	if strings.Contains(prompt, "Do not guess exact answers when sandbox_exec can verify them.") {
		t.Fatalf("no-tools prompt includes sandbox_exec verification rule:\n%s", prompt)
	}
}

func TestBuildSystemPromptDoesNotPreferSandboxExecWhenUnavailable(t *testing.T) {
	prompt := buildSystemPrompt([]outbound.ToolDefinition{
		{Name: "workspace", Description: "Lists or stats workspace paths without reading file contents."},
	})

	if strings.Contains(prompt, "call sandbox_exec before final") {
		t.Fatalf("prompt prefers sandbox_exec when it is unavailable:\n%s", prompt)
	}
	if strings.Contains(prompt, "Do not guess exact answers when sandbox_exec can verify them.") {
		t.Fatalf("prompt includes sandbox_exec verification rule when it is unavailable:\n%s", prompt)
	}
}

func TestBuildSystemPromptStaysConcise(t *testing.T) {
	prompt := buildSystemPrompt(testPromptTools())

	if len(prompt) > 3500 {
		t.Fatalf("len(prompt) = %d, want <= 3500:\n%s", len(prompt), prompt)
	}
}

func testPromptTools() []outbound.ToolDefinition {
	return []outbound.ToolDefinition{
		{Name: "echo", Description: "Returns text"},
		{
			Name:        "workspace",
			Description: "Lists or stats workspace paths without reading file contents.",
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
			Description: "Runs a command inside the sandbox from /workspace/work.",
			Arguments: []outbound.ToolArgumentDefinition{
				{
					Name:        "command",
					Description: "Command to run inside the sandbox.",
					Required:    true,
					Example:     "wc -l /workspace/input/README.md",
				},
				{
					Name:        "timeout_seconds",
					Description: "Optional timeout in seconds. Must be a positive integer and no more than the configured maximum.",
					Required:    false,
					Example:     "10",
				},
			},
		},
	}
}

func assertPromptContains(t *testing.T, prompt string, want string) {
	t.Helper()

	if !strings.Contains(prompt, want) {
		t.Fatalf("prompt does not contain %q:\n%s", want, prompt)
	}
}

func assertNoOldToolNames(t *testing.T, prompt string) {
	t.Helper()

	for _, oldToolName := range []string{"list_" + "files", "read_" + "file", "run_" + "shell"} {
		if strings.Contains(prompt, oldToolName) {
			t.Fatalf("prompt contains old tool name %q:\n%s", oldToolName, prompt)
		}
	}
}
