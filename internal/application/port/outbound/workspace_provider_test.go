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
		Source: run.WorkspaceSource{Type: run.WorkspaceSourceConfigured},
	})
	if err != nil {
		t.Fatalf("ResolveWorkspace() error = %v", err)
	}
	if workspace.Path != "/tmp/repo" {
		t.Fatalf("workspace path = %q, want /tmp/repo", workspace.Path)
	}
}

type contractWorkspaceProvider struct{}

func (contractWorkspaceProvider) ResolveWorkspace(context.Context, WorkspaceResolveRequest) (Workspace, error) {
	return Workspace{Path: "/tmp/repo"}, nil
}
