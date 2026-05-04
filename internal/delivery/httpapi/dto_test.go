package httpapi

import (
	"encoding/json"
	"strings"
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
			Workspace:     inbound.WorkspaceSourceView{Type: "none"},
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

	response := toRunResponse(view)

	if response.ID != view.ID {
		t.Fatalf("ID = %s, want %s", response.ID, view.ID)
	}
	if response.Task.RepositoryURL != view.Task.RepositoryURL {
		t.Fatalf("repository URL = %s, want %s", response.Task.RepositoryURL, view.Task.RepositoryURL)
	}
	if response.Task.Workspace != nil {
		t.Fatalf("Workspace = %#v, want nil", response.Task.Workspace)
	}
	if response.Result == nil || response.Result.Summary != view.Result.Summary {
		t.Fatalf("Result = %#v, want summary %q", response.Result, view.Result.Summary)
	}
	if response.FailureReason != view.FailureReason {
		t.Fatalf("FailureReason = %q, want %q", response.FailureReason, view.FailureReason)
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

func TestRunResponseJSONIncludesCompletedResultSummary(t *testing.T) {
	payload, err := json.Marshal(toRunResponse(inbound.RunView{
		ID:        "run_test",
		Status:    "completed",
		Result:    inbound.RunResultView{Summary: "model output"},
		Steps:     []inbound.StepView{},
		CreatedAt: time.Unix(100, 0).UTC(),
		UpdatedAt: time.Unix(101, 0).UTC(),
	}))
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	got := string(payload)
	if !strings.Contains(got, `"result":{"summary":"model output"}`) {
		t.Fatalf("response does not contain result summary: %s", got)
	}
}

func TestRunResponseJSONIncludesCompletedSteps(t *testing.T) {
	endedAt := time.Unix(102, 0).UTC()
	payload, err := json.Marshal(toRunResponse(inbound.RunView{
		ID:     "run_test",
		Status: "completed",
		Steps: []inbound.StepView{
			{
				Name:      "prepare",
				Status:    "completed",
				Message:   "Prepared policy, secrets, and source context",
				StartedAt: time.Unix(101, 0).UTC(),
				EndedAt:   &endedAt,
			},
			{
				Name:      "agent",
				Status:    "completed",
				Message:   "Agent generated result summary",
				StartedAt: time.Unix(101, 0).UTC(),
				EndedAt:   &endedAt,
			},
		},
		CreatedAt: time.Unix(100, 0).UTC(),
		UpdatedAt: time.Unix(103, 0).UTC(),
	}))
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	got := string(payload)
	if !strings.Contains(got, `"name":"prepare"`) {
		t.Fatalf("response does not contain prepare step: %s", got)
	}
	if !strings.Contains(got, `"message":"Agent generated result summary"`) {
		t.Fatalf("response does not contain agent step message: %s", got)
	}
	if !strings.Contains(got, `"ended_at"`) {
		t.Fatalf("response does not contain ended_at for completed steps: %s", got)
	}
}

func TestRunResponseJSONIncludesFailureReason(t *testing.T) {
	payload, err := json.Marshal(toRunResponse(inbound.RunView{
		ID:            "run_test",
		Status:        "failed",
		FailureReason: "model generation failed",
		Steps:         []inbound.StepView{},
		CreatedAt:     time.Unix(100, 0).UTC(),
		UpdatedAt:     time.Unix(101, 0).UTC(),
	}))
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	got := string(payload)
	if !strings.Contains(got, `"failure_reason":"model generation failed"`) {
		t.Fatalf("response does not contain failure reason: %s", got)
	}
}

func TestRunResponseJSONIncludesFailedStep(t *testing.T) {
	endedAt := time.Unix(102, 0).UTC()
	payload, err := json.Marshal(toRunResponse(inbound.RunView{
		ID:            "run_test",
		Status:        "failed",
		FailureReason: "run failed",
		Steps: []inbound.StepView{
			{
				Name:      "agent",
				Status:    "failed",
				Message:   "Agent execution failed",
				StartedAt: time.Unix(101, 0).UTC(),
				EndedAt:   &endedAt,
			},
		},
		CreatedAt: time.Unix(100, 0).UTC(),
		UpdatedAt: time.Unix(103, 0).UTC(),
	}))
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	got := string(payload)
	if !strings.Contains(got, `"failure_reason":"run failed"`) {
		t.Fatalf("response does not contain failure reason: %s", got)
	}
	if !strings.Contains(got, `"name":"agent"`) {
		t.Fatalf("response does not contain agent step: %s", got)
	}
	if !strings.Contains(got, `"status":"failed"`) {
		t.Fatalf("response does not contain failed step status: %s", got)
	}
}

func TestRunResponseJSONOmitsEmptyResultAndFailureReason(t *testing.T) {
	payload, err := json.Marshal(toRunResponse(inbound.RunView{
		ID:        "run_test",
		Status:    "queued",
		Steps:     []inbound.StepView{},
		CreatedAt: time.Unix(100, 0).UTC(),
		UpdatedAt: time.Unix(101, 0).UTC(),
	}))
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	got := string(payload)
	if strings.Contains(got, `"result"`) {
		t.Fatalf("response contains empty result: %s", got)
	}
	if strings.Contains(got, `"failure_reason"`) {
		t.Fatalf("response contains empty failure reason: %s", got)
	}
}

func TestRunResponseJSONOmitsNoneWorkspace(t *testing.T) {
	payload, err := json.Marshal(toRunResponse(inbound.RunView{
		ID:     "run_test",
		Status: "queued",
		Task: inbound.TaskView{
			Prompt:    "do work",
			Workspace: inbound.WorkspaceSourceView{Type: "none"},
		},
		Steps:     []inbound.StepView{},
		CreatedAt: time.Unix(100, 0).UTC(),
		UpdatedAt: time.Unix(101, 0).UTC(),
	}))
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	got := string(payload)
	if strings.Contains(got, `"workspace"`) {
		t.Fatalf("response contains none workspace: %s", got)
	}
}
