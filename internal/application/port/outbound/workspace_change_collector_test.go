package outbound

import (
	"context"
	"testing"
)

func TestWorkspaceChangeCollectorContract(t *testing.T) {
	var collector WorkspaceChangeCollector = contractWorkspaceChangeCollector{}

	changes, err := collector.CollectWorkspaceChanges(context.Background(), Workspace{
		Path:     "/tmp/current",
		BasePath: "/tmp/base",
	})
	if err != nil {
		t.Fatalf("CollectWorkspaceChanges() error = %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("len(changes) = %d, want 1", len(changes))
	}
	if changes[0].Status != WorkspaceChangeModified {
		t.Fatalf("changes[0].Status = %q, want modified", changes[0].Status)
	}
}

type contractWorkspaceChangeCollector struct{}

func (contractWorkspaceChangeCollector) CollectWorkspaceChanges(context.Context, Workspace) ([]WorkspaceChange, error) {
	return []WorkspaceChange{{Path: "README.md", Status: WorkspaceChangeModified}}, nil
}
