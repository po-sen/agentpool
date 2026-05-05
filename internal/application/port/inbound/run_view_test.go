package inbound

import (
	"testing"
	"time"
)

func TestRunViewCarriesApplicationOutputFields(t *testing.T) {
	endedAt := time.Unix(102, 0).UTC()

	view := RunView{
		ID:     "run_test",
		Status: "running",
		Task: TaskView{
			ProjectID:     "project_test",
			Prompt:        "do work",
			RepositoryURL: "https://example.com/repo.git",
			Branch:        "main",
			Attachments: []AttachmentView{
				{Filename: "README.md", MediaType: "text/markdown", SizeBytes: 7},
			},
		},
		Result: RunResultView{
			Summary: "model output",
		},
		FailureReason:     "model failed",
		FailureCode:       "model_generate_failed",
		FailureMessage:    "model generation failed",
		AgentSystemPrompt: "system prompt",
		Steps: []StepView{
			{
				Name:      "execute",
				Status:    "completed",
				Message:   "done",
				StartedAt: time.Unix(101, 0).UTC(),
				EndedAt:   &endedAt,
			},
		},
		ToolCalls: []ToolCallView{
			{
				Name:      "workspace",
				Arguments: map[string]string{"operation": "stat", "area": "input", "path": "README.md"},
				Result:    "virtual_path: /workspace/input/README.md\n",
				StartedAt: time.Unix(104, 0).UTC(),
				EndedAt:   time.Unix(105, 0).UTC(),
			},
		},
		AgentTurns: []AgentTurnView{
			{
				Index:           1,
				Status:          "tool_call",
				ActionType:      "tool_call",
				ToolName:        "workspace",
				Message:         "model requested tool call",
				ResponsePreview: `{"type":"tool_call"}`,
				StartedAt:       time.Unix(106, 0).UTC(),
				EndedAt:         time.Unix(107, 0).UTC(),
			},
		},
		Artifacts: []ArtifactView{
			{Path: "report.md", MediaType: "text/markdown", SizeBytes: 9},
		},
		CreatedAt: time.Unix(100, 0).UTC(),
		UpdatedAt: time.Unix(103, 0).UTC(),
	}

	if view.ID != "run_test" {
		t.Fatalf("ID = %s, want run_test", view.ID)
	}
	if view.Task.Branch != "main" {
		t.Fatalf("Branch = %s, want main", view.Task.Branch)
	}
	if view.Task.Attachments[0].Filename != "README.md" {
		t.Fatalf("Attachments[0].Filename = %q, want README.md", view.Task.Attachments[0].Filename)
	}
	if view.Result.Summary != "model output" {
		t.Fatalf("Result.Summary = %q, want model output", view.Result.Summary)
	}
	if view.FailureReason != "model failed" {
		t.Fatalf("FailureReason = %q, want model failed", view.FailureReason)
	}
	if view.FailureCode != "model_generate_failed" {
		t.Fatalf("FailureCode = %q, want model_generate_failed", view.FailureCode)
	}
	if view.FailureMessage != "model generation failed" {
		t.Fatalf("FailureMessage = %q, want model generation failed", view.FailureMessage)
	}
	if view.AgentSystemPrompt != "system prompt" {
		t.Fatalf("AgentSystemPrompt = %q, want system prompt", view.AgentSystemPrompt)
	}
	if view.Steps[0].EndedAt == nil || !view.Steps[0].EndedAt.Equal(endedAt) {
		t.Fatalf("EndedAt = %v, want %v", view.Steps[0].EndedAt, endedAt)
	}
	if view.ToolCalls[0].Arguments["operation"] != "stat" {
		t.Fatalf("ToolCalls[0] operation = %q, want stat", view.ToolCalls[0].Arguments["operation"])
	}
	if view.AgentTurns[0].ToolName != "workspace" {
		t.Fatalf("AgentTurns[0].ToolName = %q, want workspace", view.AgentTurns[0].ToolName)
	}
	if view.Artifacts[0].Path != "report.md" {
		t.Fatalf("Artifacts[0].Path = %q, want report.md", view.Artifacts[0].Path)
	}

	content := ArtifactContentView{Path: "report.md", Content: []byte("# Report\n"), SizeBytes: 9}
	if string(content.Content) != "# Report\n" {
		t.Fatalf("ArtifactContentView.Content = %q, want report", string(content.Content))
	}
}
