package run

import (
	"testing"
)

func TestStatusValidity(t *testing.T) {
	valid := []Status{
		StatusQueued,
		StatusPreparing,
		StatusRunning,
		StatusWaitingApproval,
		StatusCompleted,
		StatusFailed,
		StatusCancelled,
	}

	for _, status := range valid {
		if !status.IsValid() {
			t.Fatalf("%s should be valid", status)
		}
	}

	if Status("unknown").IsValid() {
		t.Fatal("unknown status should be invalid")
	}
}

func TestStatusTerminal(t *testing.T) {
	terminal := []Status{
		StatusCompleted,
		StatusFailed,
		StatusCancelled,
	}

	for _, status := range terminal {
		if !status.IsTerminal() {
			t.Fatalf("%s should be terminal", status)
		}
	}

	if StatusRunning.IsTerminal() {
		t.Fatal("running should not be terminal")
	}
}
