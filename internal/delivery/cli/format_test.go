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
		Artifacts: []ArtifactResponse{
			{Path: "report.md", MediaType: "text/markdown", SizeBytes: 9},
		},
		Steps: []StepResponse{
			{Name: "workspace", Status: statusCompleted, Message: "Prepared empty workspace"},
			{Name: "agent", Status: statusCompleted, Message: "Agent generated result summary"},
		},
		ToolCalls: []ToolCallResponse{{Name: "sandbox_exec"}},
	}, OutputOptions{})

	for _, want := range []string{"Run: run_done", "Status: completed", "234 * 887123", "Artifacts:", "- report.md (9 bytes, text/markdown)", "Tool calls:", "- sandbox_exec: ok"} {
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

func TestFormatRunDebugIncludesPromptMetadataAndDetails(t *testing.T) {
	output := FormatRun(RunResponse{
		ID:                        "run_debug",
		Status:                    statusCompleted,
		AgentPromptVersion:        "agentpool-runtime-v1",
		AgentPromptSHA256:         "abc123",
		AgentSystemPromptRedacted: true,
		AgentTurns: []AgentTurnResponse{
			{
				Index:             1,
				Status:            "protocol_error",
				ResponseFormat:    "plain_text",
				ProtocolErrorCode: "invalid_json",
				CorrectionMessage: "Protocol error:\nReturn exactly one JSON object.",
				RequestMessages:   []MessageResponse{{Role: "user", Content: "do work"}},
				RawResponse:       "done",
				ResponsePreview:   "done",
			},
		},
		ToolCalls: []ToolCallResponse{
			{Name: "workspace", Arguments: map[string]string{"operation": "list", "path": "."}, Result: "/workspace/input/README.md"},
		},
	}, OutputOptions{Debug: true})

	for _, want := range []string{"Agent prompt:", "version: agentpool-runtime-v1", "sha256: abc123", "redacted: true", "response_format: plain_text", "protocol_error_code: invalid_json", "correction_message: Protocol error:", "request_messages:", "1. user: do work", "raw_response: done", "response_preview:", "operation: list", "path: .", "result: /workspace/input/README.md"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "AgentPool is running a task.") {
		t.Fatalf("debug output exposed raw system prompt:\n%s", output)
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

func TestFormatArtifacts(t *testing.T) {
	output := FormatArtifacts(ArtifactsResponse{
		Artifacts: []ArtifactResponse{{Path: "report.md", MediaType: "text/markdown", SizeBytes: 9}},
	})
	if !strings.Contains(output, "- report.md (9 bytes, text/markdown)") {
		t.Fatalf("artifact output missing report: %s", output)
	}
	if got := FormatArtifacts(ArtifactsResponse{}); got != "Artifacts: none\n" {
		t.Fatalf("empty artifact output = %q, want none", got)
	}
}
