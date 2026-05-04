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
			Workspace:     WorkspaceSourceView{Type: "snapshot", SnapshotID: "wsnap_test"},
		},
		Result: RunResultView{
			Summary: "model output",
		},
		FailureReason: "model failed",
		Steps: []StepView{
			{
				Name:      "execute",
				Status:    "completed",
				Message:   "done",
				StartedAt: time.Unix(101, 0).UTC(),
				EndedAt:   &endedAt,
			},
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
	if view.Task.Workspace.Type != "snapshot" {
		t.Fatalf("Workspace.Type = %s, want snapshot", view.Task.Workspace.Type)
	}
	if view.Task.Workspace.SnapshotID != "wsnap_test" {
		t.Fatalf("Workspace.SnapshotID = %s, want wsnap_test", view.Task.Workspace.SnapshotID)
	}
	if view.Result.Summary != "model output" {
		t.Fatalf("Result.Summary = %q, want model output", view.Result.Summary)
	}
	if view.FailureReason != "model failed" {
		t.Fatalf("FailureReason = %q, want model failed", view.FailureReason)
	}
	if view.Steps[0].EndedAt == nil || !view.Steps[0].EndedAt.Equal(endedAt) {
		t.Fatalf("EndedAt = %v, want %v", view.Steps[0].EndedAt, endedAt)
	}
}
