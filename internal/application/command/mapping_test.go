package command

import (
	"testing"
	"time"

	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestToRunViewMapsRunAggregate(t *testing.T) {
	endedAt := time.Unix(102, 0).UTC()
	item, err := run.New("run_test", run.TaskSpec{
		ProjectID:     "project_test",
		Prompt:        "do work",
		RepositoryURL: "https://example.com/repo.git",
		Branch:        "main",
		Workspace: run.WorkspaceSource{
			Type: run.WorkspaceSourceConfigured,
		},
	}, time.Unix(100, 0).UTC())
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	item.Status = run.StatusRunning
	item.ResultSummary = "model output"
	item.FailureReason = "model failed"
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
	if view.Task.Workspace.Type != string(run.WorkspaceSourceConfigured) {
		t.Fatalf("Workspace.Type = %s, want configured", view.Task.Workspace.Type)
	}
	if view.Result.Summary != item.ResultSummary {
		t.Fatalf("Result.Summary = %q, want %q", view.Result.Summary, item.ResultSummary)
	}
	if view.FailureReason != item.FailureReason {
		t.Fatalf("FailureReason = %q, want %q", view.FailureReason, item.FailureReason)
	}
	if len(view.Steps) != 1 {
		t.Fatalf("len(Steps) = %d, want 1", len(view.Steps))
	}
	if view.Steps[0].EndedAt == nil || !view.Steps[0].EndedAt.Equal(endedAt) {
		t.Fatalf("EndedAt = %v, want %v", view.Steps[0].EndedAt, endedAt)
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
