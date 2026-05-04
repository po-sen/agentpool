package noop

import (
	"context"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestProviderPrepareAndCleanup(t *testing.T) {
	provider := NewProvider()

	prepared, err := provider.Prepare(context.Background(), outbound.SandboxRequest{
		RunID: "run_test",
	})
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if prepared.ID == "" {
		t.Fatal("prepared sandbox ID is empty")
	}
	if err := provider.Cleanup(context.Background(), prepared); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
}

func TestProviderRunCommandReportsUnavailable(t *testing.T) {
	provider := NewProvider()

	result, err := provider.RunCommand(context.Background(), outbound.SandboxCommandRequest{
		Sandbox: outbound.Sandbox{ID: "noop"},
		Command: "echo hello",
	})
	if err != nil {
		t.Fatalf("RunCommand() error = %v", err)
	}
	if result.ExitCode == 0 {
		t.Fatalf("ExitCode = %d, want non-zero", result.ExitCode)
	}
	if result.Stderr != "sandbox command execution is not available" {
		t.Fatalf("Stderr = %q, want unavailable message", result.Stderr)
	}
}
