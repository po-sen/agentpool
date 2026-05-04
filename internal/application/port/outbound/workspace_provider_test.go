package outbound

import (
	"context"
	"testing"

	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestWorkspaceProviderContract(t *testing.T) {
	var provider WorkspaceProvider = contractWorkspaceProvider{}

	workspace, err := provider.PrepareWorkspace(context.Background(), WorkspacePrepareRequest{
		RunID: "run_test",
		Attachments: []run.TaskAttachment{
			{Filename: "README.md", Content: []byte("# Demo\n"), SizeBytes: 7},
		},
	})
	if err != nil {
		t.Fatalf("PrepareWorkspace() error = %v", err)
	}
	if workspace.Path == "" {
		t.Fatal("workspace path is empty")
	}
	if err := provider.CleanupWorkspace(context.Background(), workspace); err != nil {
		t.Fatalf("CleanupWorkspace() error = %v", err)
	}
}

type contractWorkspaceProvider struct{}

func (contractWorkspaceProvider) PrepareWorkspace(context.Context, WorkspacePrepareRequest) (Workspace, error) {
	return Workspace{Path: "/tmp/workspace"}, nil
}

func (contractWorkspaceProvider) CleanupWorkspace(context.Context, Workspace) error {
	return nil
}
