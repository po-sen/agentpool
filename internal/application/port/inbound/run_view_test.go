package inbound_test

import (
	"testing"
	"time"

	"github.com/po-sen/agentpool/internal/application/port/inbound"
)

func TestRunViewCarriesApplicationOutputFields(t *testing.T) {
	endedAt := time.Unix(102, 0).UTC()

	view := inbound.RunView{
		ID:     "run_test",
		Status: "running",
		Task: inbound.TaskView{
			ProjectID:     "project_test",
			Prompt:        "do work",
			RepositoryURL: "https://example.com/repo.git",
			Branch:        "main",
		},
		Result: inbound.RunResultView{
			Summary: "model output",
		},
		FailureReason: "model failed",
		Steps: []inbound.StepView{
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
