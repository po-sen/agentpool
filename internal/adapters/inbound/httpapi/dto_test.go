package httpapi

import (
	"testing"
	"time"

	"github.com/po-sen/agentpool/internal/application/port/inbound"
)

func TestToRunResponseMapsApplicationView(t *testing.T) {
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

	response := toRunResponse(view)

	if response.ID != view.ID {
		t.Fatalf("ID = %s, want %s", response.ID, view.ID)
	}
	if response.Task.RepositoryURL != view.Task.RepositoryURL {
		t.Fatalf("repository URL = %s, want %s", response.Task.RepositoryURL, view.Task.RepositoryURL)
	}
	if len(response.Steps) != 1 {
		t.Fatalf("len(Steps) = %d, want 1", len(response.Steps))
	}
	if response.Steps[0].EndedAt == nil || !response.Steps[0].EndedAt.Equal(endedAt) {
		t.Fatalf("EndedAt = %v, want %v", response.Steps[0].EndedAt, endedAt)
	}
}

func TestToRunResponsePreservesNilStepEndedAt(t *testing.T) {
	response := toRunResponse(inbound.RunView{
		ID:     "run_test",
		Status: "running",
		Steps: []inbound.StepView{
			{
				Name:      "execute",
				Status:    "running",
				StartedAt: time.Unix(100, 0).UTC(),
			},
		},
	})

	if len(response.Steps) != 1 {
		t.Fatalf("len(Steps) = %d, want 1", len(response.Steps))
	}
	if response.Steps[0].EndedAt != nil {
		t.Fatalf("EndedAt = %v, want nil", response.Steps[0].EndedAt)
	}
}
