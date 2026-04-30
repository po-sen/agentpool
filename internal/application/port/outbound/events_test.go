package outbound_test

import (
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestRunLifecycleEventNames(t *testing.T) {
	tests := map[string]string{
		"created":   outbound.EventRunCreated,
		"preparing": outbound.EventRunPreparing,
		"started":   outbound.EventRunStarted,
		"completed": outbound.EventRunCompleted,
		"failed":    outbound.EventRunFailed,
		"cancelled": outbound.EventRunCancelled,
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
