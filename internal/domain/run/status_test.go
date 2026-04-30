package run_test

import (
	"testing"

	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestStatusValidity(t *testing.T) {
	valid := []run.Status{
		run.StatusQueued,
		run.StatusPreparing,
		run.StatusRunning,
		run.StatusWaitingApproval,
		run.StatusCompleted,
		run.StatusFailed,
		run.StatusCancelled,
	}

	for _, status := range valid {
		if !status.IsValid() {
			t.Fatalf("%s should be valid", status)
		}
	}

	if run.Status("unknown").IsValid() {
		t.Fatal("unknown status should be invalid")
	}
}

func TestStatusTerminal(t *testing.T) {
	terminal := []run.Status{
		run.StatusCompleted,
		run.StatusFailed,
		run.StatusCancelled,
	}

	for _, status := range terminal {
		if !status.IsTerminal() {
			t.Fatalf("%s should be terminal", status)
		}
	}

	if run.StatusRunning.IsTerminal() {
		t.Fatal("running should not be terminal")
	}
}
