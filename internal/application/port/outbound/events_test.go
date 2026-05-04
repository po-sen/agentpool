package outbound

import (
	"testing"
)

func TestRunLifecycleEventNames(t *testing.T) {
	tests := map[string]string{
		"created":   EventRunCreated,
		"preparing": EventRunPreparing,
		"started":   EventRunStarted,
		"completed": EventRunCompleted,
		"failed":    EventRunFailed,
		"cancelled": EventRunCancelled,
	}

	want := map[string]string{
		"created":   "run.created",
		"preparing": "run.preparing",
		"started":   "run.started",
		"completed": "run.completed",
		"failed":    "run.failed",
		"cancelled": "run.cancelled",
	}

	for name, got := range tests {
		if got != want[name] {
			t.Fatalf("%s event = %q, want %q", name, got, want[name])
		}
	}
}
