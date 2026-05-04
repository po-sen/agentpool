package outbound

import (
	"context"
	"testing"

	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestWorkspaceProviderContract(t *testing.T) {
	var provider WorkspaceProvider = contractWorkspaceProvider{}
	var collector WorkspaceChangeCollector = contractWorkspaceChangeCollector{}

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
	changes, err := collector.CollectWorkspaceChanges(context.Background(), workspace)
	if err != nil {
		t.Fatalf("CollectWorkspaceChanges() error = %v", err)
	}
	if len(changes) != 1 || changes[0].Status != WorkspaceChangeModified {
		t.Fatalf("changes = %#v, want modified change", changes)
	}
}

type contractWorkspaceProvider struct{}

func (contractWorkspaceProvider) ResolveWorkspace(context.Context, WorkspaceResolveRequest) (Workspace, error) {
	return Workspace{}, nil
}

func (contractWorkspaceProvider) CleanupWorkspace(context.Context, Workspace) error {
	return nil
}

type contractWorkspaceChangeCollector struct{}

func (contractWorkspaceChangeCollector) CollectWorkspaceChanges(context.Context, Workspace) ([]WorkspaceChange, error) {
	return []WorkspaceChange{{Path: "README.md", Status: WorkspaceChangeModified}}, nil
}
