package outbound

import (
	"context"
	"testing"

	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestWorkspaceProviderContract(t *testing.T) {
	var provider WorkspaceProvider = contractWorkspaceProvider{}

	workspace, err := provider.ResolveWorkspace(context.Background(), WorkspaceResolveRequest{
		RunID:  "run_test",
		Source: run.WorkspaceSource{Type: run.WorkspaceSourceNone},
	})
	if err != nil {
		t.Fatalf("ResolveWorkspace() error = %v", err)
	}
	if workspace.Path != "" {
		t.Fatalf("workspace path = %q, want empty", workspace.Path)
	}
	if err := provider.CleanupWorkspace(context.Background(), workspace); err != nil {
		t.Fatalf("CleanupWorkspace() error = %v", err)
	}
}

type contractWorkspaceProvider struct{}

func (contractWorkspaceProvider) ResolveWorkspace(context.Context, WorkspaceResolveRequest) (Workspace, error) {
	return Workspace{}, nil
}

func (contractWorkspaceProvider) CleanupWorkspace(context.Context, Workspace) error {
	return nil
}
