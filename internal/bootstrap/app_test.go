package bootstrap_test

import (
	"context"
	"io"
	"testing"

	"github.com/po-sen/agentpool/internal/bootstrap"
)

func TestNewWiresVersion(t *testing.T) {
	t.Setenv("AGENTPOOL_MODEL_PROVIDER", "noop")

	app, err := bootstrap.New("test-version", io.Discard)
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

	app, err := bootstrap.New("test-version", io.Discard)
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

	app, err := bootstrap.New("test-version", io.Discard)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if got, want := app.Version(), "agentpool test-version"; got != want {
		t.Fatalf("Version() = %q, want %q", got, want)
	}
}

func TestRunWorkerReturnsErrorForInvalidModelProvider(t *testing.T) {
	t.Setenv("AGENTPOOL_MODEL_PROVIDER", "bad")

	app, err := bootstrap.New("test-version", io.Discard)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := app.RunWorker(context.Background()); err == nil {
		t.Fatal("RunWorker() error = nil, want error")
	}
}
