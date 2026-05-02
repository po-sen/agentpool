package noop_test

import (
	"context"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	sandboxnoop "github.com/po-sen/agentpool/internal/infrastructure/sandbox/noop"
)

func TestProviderPrepareAndCleanup(t *testing.T) {
	provider := sandboxnoop.NewProvider()

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
