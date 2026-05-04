package bootstrap

import (
	"context"
	"io"
	"path/filepath"
	"testing"
)

func TestNewWiresVersion(t *testing.T) {
	t.Setenv("AGENTPOOL_MODEL_PROVIDER", "noop")

	app, err := New("test-version", io.Discard)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if got, want := app.Version(), "agentpool test-version"; got != want {
		t.Fatalf("Version() = %q, want %q", got, want)
	}
}

func TestRunWorkerBuildsOpenAICompatibleModelProvider(t *testing.T) {
	t.Setenv("AGENTPOOL_MODEL_PROVIDER", "openai_compatible")
	t.Setenv("AGENTPOOL_MODEL_BASE_URL", "http://localhost:11434/v1")
	t.Setenv("AGENTPOOL_MODEL_NAME", "local-model")
	t.Setenv("AGENTPOOL_WORKSPACE_PATH", "")

	app, err := New("test-version", io.Discard)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := app.RunWorker(ctx); err != nil {
		t.Fatalf("RunWorker() error = %v", err)
	}
}

func TestNewDoesNotValidateWorkerModelProvider(t *testing.T) {
	t.Setenv("AGENTPOOL_MODEL_PROVIDER", "openai")
	t.Setenv("AGENTPOOL_MODEL_API_KEY", "")

	app, err := New("test-version", io.Discard)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if got, want := app.Version(), "agentpool test-version"; got != want {
		t.Fatalf("Version() = %q, want %q", got, want)
	}
}

func TestNewDoesNotValidateWorkspacePath(t *testing.T) {
	t.Setenv("AGENTPOOL_MODEL_PROVIDER", "noop")
	t.Setenv("AGENTPOOL_WORKSPACE_PATH", filepath.Join(t.TempDir(), "missing"))

	app, err := New("test-version", io.Discard)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if got, want := app.Version(), "agentpool test-version"; got != want {
		t.Fatalf("Version() = %q, want %q", got, want)
	}
}

func TestRunWorkerReturnsErrorForInvalidModelProvider(t *testing.T) {
	t.Setenv("AGENTPOOL_MODEL_PROVIDER", "bad")

	app, err := New("test-version", io.Discard)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := app.RunWorker(context.Background()); err == nil {
		t.Fatal("RunWorker() error = nil, want error")
	}
}

func TestRunWorkerDoesNotValidateUnusedWorkspacePath(t *testing.T) {
	t.Setenv("AGENTPOOL_MODEL_PROVIDER", "noop")
	t.Setenv("AGENTPOOL_WORKSPACE_PATH", filepath.Join(t.TempDir(), "missing"))

	app, err := New("test-version", io.Discard)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := app.RunWorker(ctx); err != nil {
		t.Fatalf("RunWorker() error = %v", err)
	}
}

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
	t.Setenv("AGENTPOOL_WORKSPACE_PATH", "")

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

func TestWorkerInstanceUsesConfiguredWorkspacePath(t *testing.T) {
	t.Setenv("AGENTPOOL_MODEL_PROVIDER", "noop")
	t.Setenv("AGENTPOOL_WORKSPACE_PATH", t.TempDir())

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
