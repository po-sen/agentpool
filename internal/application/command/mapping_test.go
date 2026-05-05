package command

import (
	"testing"
	"time"

	"github.com/po-sen/agentpool/internal/application/port/inbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestToRunViewMapsRunAggregate(t *testing.T) {
	endedAt := time.Unix(102, 0).UTC()
	item, err := run.New("run_test", run.TaskSpec{
		ProjectID:     "project_test",
		Prompt:        "do work",
		RepositoryURL: "https://example.com/repo.git",
		Branch:        "main",
		Attachments: []run.TaskAttachment{
			{
				Filename:  "README.md",
				MediaType: "text/markdown",
				Content:   []byte("# Demo\n"),
				SizeBytes: 7,
			},
		},
	}, time.Unix(100, 0).UTC())
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	item.Status = run.StatusRunning
	item.ResultSummary = "model output"
	item.FailureReason = "model failed"
	item.FailureCode = run.FailureCodeModelGenerateFailed
	item.FailureMessage = "model generation failed"
	item.AgentSystemPrompt = "system prompt"
	item.ToolCalls = []run.ToolCall{
		{
			Name:      "sandbox_exec",
			Arguments: map[string]string{"command": "pwd"},
			Result:    "exit_code: 0",
			StartedAt: time.Unix(104, 0).UTC(),
			EndedAt:   time.Unix(105, 0).UTC(),
		},
	}
	item.AgentTurns = []run.AgentTurn{
		{
			Index:           1,
			Status:          run.AgentTurnStatusToolCall,
			ActionType:      run.AgentTurnActionTypeToolCall,
			ToolName:        "sandbox_exec",
			Message:         "model requested tool call",
			ResponsePreview: `{"type":"tool_call"}`,
			StartedAt:       time.Unix(106, 0).UTC(),
			EndedAt:         time.Unix(107, 0).UTC(),
		},
	}
	item.Steps = []run.Step{
		{
			Name:      "execute",
			Status:    run.StatusCompleted,
			Message:   "done",
			StartedAt: time.Unix(101, 0).UTC(),
			EndedAt:   endedAt,
		},
	}

	view := toRunView(item)

	if view.ID != item.ID.String() {
		t.Fatalf("ID = %s, want %s", view.ID, item.ID)
	}
	if view.Task.ProjectID != item.Task.ProjectID {
		t.Fatalf("ProjectID = %s, want %s", view.Task.ProjectID, item.Task.ProjectID)
	}
	if len(view.Task.Attachments) != 1 {
		t.Fatalf("len(Attachments) = %d, want 1", len(view.Task.Attachments))
	}
	if view.Task.Attachments[0].Filename != "README.md" {
		t.Fatalf("Attachments[0].Filename = %q, want README.md", view.Task.Attachments[0].Filename)
	}
	if view.Result.Summary != item.ResultSummary {
		t.Fatalf("Result.Summary = %q, want %q", view.Result.Summary, item.ResultSummary)
	}
	if view.FailureReason != item.FailureReason {
		t.Fatalf("FailureReason = %q, want %q", view.FailureReason, item.FailureReason)
	}
	if view.FailureCode != item.FailureCode {
		t.Fatalf("FailureCode = %q, want %q", view.FailureCode, item.FailureCode)
	}
	if view.FailureMessage != item.FailureMessage {
		t.Fatalf("FailureMessage = %q, want %q", view.FailureMessage, item.FailureMessage)
	}
	assertMappedAgentSystemPrompt(t, view, item)
	if len(view.Steps) != 1 {
		t.Fatalf("len(Steps) = %d, want 1", len(view.Steps))
	}
	if len(view.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(view.ToolCalls))
	}
	if view.ToolCalls[0].Arguments["command"] != "pwd" {
		t.Fatalf("ToolCalls[0] command = %q, want pwd", view.ToolCalls[0].Arguments["command"])
	}
	view.ToolCalls[0].Arguments["command"] = "changed"
	if item.ToolCalls[0].Arguments["command"] != "pwd" {
		t.Fatalf("domain tool call command = %q, want pwd", item.ToolCalls[0].Arguments["command"])
	}
	assertMappedAgentTurn(t, view.AgentTurns)
	if view.Steps[0].EndedAt == nil || !view.Steps[0].EndedAt.Equal(endedAt) {
		t.Fatalf("EndedAt = %v, want %v", view.Steps[0].EndedAt, endedAt)
	}
}

func assertMappedAgentTurn(t *testing.T, turns []inbound.AgentTurnView) {
	t.Helper()

	if len(turns) != 1 {
		t.Fatalf("len(AgentTurns) = %d, want 1", len(turns))
	}
	if turns[0].ToolName != "sandbox_exec" {
		t.Fatalf("AgentTurns[0].ToolName = %q, want sandbox_exec", turns[0].ToolName)
	}
}

func assertMappedAgentSystemPrompt(t *testing.T, view inbound.RunView, item *run.Run) {
	t.Helper()

	if view.AgentSystemPrompt != item.AgentSystemPrompt {
		t.Fatalf("AgentSystemPrompt = %q, want %q", view.AgentSystemPrompt, item.AgentSystemPrompt)
	}
}

func TestToRunViewKeepsUnfinishedStepEndNil(t *testing.T) {
	item, err := run.New("run_test", run.TaskSpec{Prompt: "do work"}, time.Unix(100, 0).UTC())
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	item.Steps = []run.Step{
		{
			Name:      "execute",
			Status:    run.StatusRunning,
			StartedAt: time.Unix(101, 0).UTC(),
		},
	}

	view := toRunView(item)
	if len(view.Steps) != 1 {
		t.Fatalf("len(Steps) = %d, want 1", len(view.Steps))
	}
	if view.Steps[0].EndedAt != nil {
		t.Fatalf("EndedAt = %v, want nil", view.Steps[0].EndedAt)
	}
}
