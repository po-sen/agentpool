package run_test

import (
	"testing"
	"time"

	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestStepCapturesExecutionProgress(t *testing.T) {
	startedAt := time.Unix(100, 0).UTC()
	endedAt := time.Unix(101, 0).UTC()

	step := run.Step{
		Name:      "execute",
		Status:    run.StatusCompleted,
		Message:   "done",
		StartedAt: startedAt,
		EndedAt:   endedAt,
	}

	if step.Name != "execute" {
		t.Fatalf("Name = %s, want execute", step.Name)
	}
	if step.Status != run.StatusCompleted {
		t.Fatalf("Status = %s, want %s", step.Status, run.StatusCompleted)
	}
	if !step.EndedAt.Equal(endedAt) {
		t.Fatalf("EndedAt = %v, want %v", step.EndedAt, endedAt)
	}
}
