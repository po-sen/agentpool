package query

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
	item.FailureCode = run.FailureCodeToolExecutionFailed
	item.FailureMessage = "tool execution failed"
	item.AgentSystemPrompt = "system prompt"
	item.ToolCalls = []run.ToolCall{
		{
			Name:      "workspace",
			Arguments: map[string]string{"operation": "stat", "area": "input", "path": "README.md"},
			Result:    "virtual_path: /workspace/input/README.md\n",
			StartedAt: time.Unix(104, 0).UTC(),
			EndedAt:   time.Unix(105, 0).UTC(),
		},
	}
	item.AgentTurns = []run.AgentTurn{
		{
			Index:           1,
			Status:          run.AgentTurnStatusToolCall,
			ActionType:      run.AgentTurnActionTypeToolCall,
			ToolName:        "workspace",
			Message:         "model requested tool call",
			RequestMessages: []run.AgentTurnMessage{{Role: "user", Content: "do work"}},
			RawResponse:     `{"type":"tool_call"}`,
			ResponseFormat:  "json_object",
			ResponsePreview: `{"type":"tool_call"}`,
			StartedAt:       time.Unix(106, 0).UTC(),
			EndedAt:         time.Unix(107, 0).UTC(),
		},
	}
	item.Artifacts = []run.Artifact{
		{Path: "report.md", MediaType: "text/markdown", Content: []byte("# Report\n"), SizeBytes: 9},
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
	if view.Task.RepositoryURL != item.Task.RepositoryURL {
		t.Fatalf("RepositoryURL = %s, want %s", view.Task.RepositoryURL, item.Task.RepositoryURL)
	}
	if len(view.Task.Attachments) != 1 {
		t.Fatalf("len(Attachments) = %d, want 1", len(view.Task.Attachments))
	}
	if view.Task.Attachments[0].SizeBytes != 7 {
		t.Fatalf("Attachments[0].SizeBytes = %d, want 7", view.Task.Attachments[0].SizeBytes)
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
	if view.ToolCalls[0].Arguments["path"] != "README.md" {
		t.Fatalf("ToolCalls[0] path = %q, want README.md", view.ToolCalls[0].Arguments["path"])
	}
	view.ToolCalls[0].Arguments["path"] = "changed.md"
	if item.ToolCalls[0].Arguments["path"] != "README.md" {
		t.Fatalf("domain tool call path = %q, want README.md", item.ToolCalls[0].Arguments["path"])
	}
	assertMappedAgentTurn(t, view.AgentTurns)
	assertMappedArtifacts(t, view.Artifacts)
	if view.Steps[0].EndedAt == nil || !view.Steps[0].EndedAt.Equal(endedAt) {
		t.Fatalf("EndedAt = %v, want %v", view.Steps[0].EndedAt, endedAt)
	}
}

func assertMappedArtifacts(t *testing.T, artifacts []inbound.ArtifactView) {
	t.Helper()

	if len(artifacts) != 1 || artifacts[0].Path != "report.md" {
		t.Fatalf("Artifacts = %#v, want report metadata", artifacts)
	}
}

func assertMappedAgentTurn(t *testing.T, turns []inbound.AgentTurnView) {
	t.Helper()

	if len(turns) != 1 {
		t.Fatalf("len(AgentTurns) = %d, want 1", len(turns))
	}
	if turns[0].Status != run.AgentTurnStatusToolCall {
		t.Fatalf("AgentTurns[0].Status = %q, want %q", turns[0].Status, run.AgentTurnStatusToolCall)
	}
	if turns[0].ResponseFormat != "json_object" {
		t.Fatalf("AgentTurns[0].ResponseFormat = %q, want json_object", turns[0].ResponseFormat)
	}
	if turns[0].RawResponse != `{"type":"tool_call"}` {
		t.Fatalf("AgentTurns[0].RawResponse = %q, want raw tool call", turns[0].RawResponse)
	}
	if len(turns[0].RequestMessages) != 1 || turns[0].RequestMessages[0].Content != "do work" {
		t.Fatalf("AgentTurns[0].RequestMessages = %#v, want mapped request messages", turns[0].RequestMessages)
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
