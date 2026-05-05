package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestFormatRunCompletedIncludesSummary(t *testing.T) {
	output := FormatRun(RunResponse{
		ID:     "run_done",
		Status: statusCompleted,
		Result: &RunResultResponse{Summary: "234 * 887123 = 207586782"},
		Steps: []StepResponse{
			{Name: "prepare", Status: statusCompleted, Message: "Prepared policy"},
			{Name: "agent", Status: statusCompleted, Message: "Agent generated result summary"},
		},
		ToolCalls: []ToolCallResponse{{Name: "sandbox_exec"}},
	}, OutputOptions{})

	for _, want := range []string{"Run: run_done", "Status: completed", "234 * 887123", "Tool calls:", "- sandbox_exec: ok"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestFormatRunFailedIncludesFailureDiagnostics(t *testing.T) {
	output := FormatRun(RunResponse{
		ID:             "run_failed",
		Status:         statusFailed,
		FailureCode:    "agent_max_turns",
		FailureMessage: "agent reached max turns",
		AgentTurns: []AgentTurnResponse{
			{Index: 1, Status: "invalid_tool_call", ToolName: "execute_shell_command"},
			{Index: 2, Status: "final"},
		},
	}, OutputOptions{})

	for _, want := range []string{"Failure: agent_max_turns - agent reached max turns", "1. invalid_tool_call execute_shell_command"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestFormatRunDebugIncludesSystemPromptAndDetails(t *testing.T) {
	output := FormatRun(RunResponse{
		ID:                "run_debug",
		Status:            statusCompleted,
		AgentSystemPrompt: "AgentPool is running a task.",
		AgentTurns: []AgentTurnResponse{
			{Index: 1, Status: "tool_call", ActionType: "tool_call", ToolName: "workspace", ResponsePreview: `{"type":"tool_call"}`},
		},
		ToolCalls: []ToolCallResponse{
			{Name: "workspace", Arguments: map[string]string{"operation": "list", "path": "."}, Result: "/workspace/input/README.md"},
		},
	}, OutputOptions{Debug: true})

	for _, want := range []string{"Agent system prompt:", "AgentPool is running a task.", "response_preview:", "operation: list", "path: .", "result: /workspace/input/README.md"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestFormatRunDefaultOmitsSystemPrompt(t *testing.T) {
	output := FormatRun(RunResponse{
		ID:                "run_default",
		Status:            statusCompleted,
		AgentSystemPrompt: "hidden prompt",
	}, OutputOptions{})

	if strings.Contains(output, "hidden prompt") {
		t.Fatalf("default output exposed system prompt:\n%s", output)
	}
}

func TestWriteRunOutputJSON(t *testing.T) {
	var buffer bytes.Buffer
	err := WriteRunOutput(&buffer, RunResponse{ID: "run_json", Status: statusCompleted}, OutputOptions{JSON: true})
	if err != nil {
		t.Fatalf("write run output: %v", err)
	}
	if !strings.Contains(buffer.String(), `"id": "run_json"`) {
		t.Fatalf("json output missing id: %s", buffer.String())
	}
}

func TestFormatRunsEmpty(t *testing.T) {
	if got := FormatRuns(nil, OutputOptions{}); got != "Runs: none\n" {
		t.Fatalf("FormatRuns() = %q, want empty list output", got)
	}
}
