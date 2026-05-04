package bootstrap

import (
	"io"
	"testing"
)

func TestNewLoadsAgentMaxTurnsConfig(t *testing.T) {
	t.Setenv("AGENTPOOL_MODEL_PROVIDER", "noop")
	t.Setenv("AGENTPOOL_AGENT_MAX_TURNS", "6")

	app, err := New("test-version", io.Discard)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if app.config.Agent.MaxTurns != 6 {
		t.Fatalf("Agent.MaxTurns = %d, want 6", app.config.Agent.MaxTurns)
	}
}

func TestWorkerInstanceWiresCompositeToolRunner(t *testing.T) {
	t.Setenv("AGENTPOOL_MODEL_PROVIDER", "noop")

	app, err := New("test-version", io.Discard)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	worker, err := app.workerInstance()
	if err != nil {
		t.Fatalf("workerInstance() error = %v", err)
	}
	if worker == nil {
		t.Fatal("workerInstance() = nil")
	}
}
