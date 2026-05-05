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
	if workspace.RootPath == "" {
		t.Fatal("workspace root path is empty")
	}
	if workspace.InputPath == "" {
		t.Fatal("workspace input path is empty")
	}
	if workspace.WorkPath == "" {
		t.Fatal("workspace work path is empty")
	}
	if !workspace.HasFiles {
		t.Fatal("workspace HasFiles = false, want true")
	}
	artifacts, err := provider.CollectArtifacts(context.Background(), workspace)
	if err != nil {
		t.Fatalf("CollectArtifacts() error = %v", err)
	}
	if len(artifacts) != 1 || artifacts[0].Path != "report.md" {
		t.Fatalf("artifacts = %#v, want report.md", artifacts)
	}
	if err := provider.CleanupWorkspace(context.Background(), workspace); err != nil {
		t.Fatalf("CleanupWorkspace() error = %v", err)
	}
}

type contractWorkspaceProvider struct{}

func (contractWorkspaceProvider) PrepareWorkspace(context.Context, WorkspacePrepareRequest) (Workspace, error) {
	return Workspace{
		RootPath:  "/tmp/workspace",
		InputPath: "/tmp/workspace/input",
		WorkPath:  "/tmp/workspace/work",
		HasFiles:  true,
	}, nil
}

func (contractWorkspaceProvider) CleanupWorkspace(context.Context, Workspace) error {
	return nil
}

func (contractWorkspaceProvider) CollectArtifacts(context.Context, Workspace) ([]run.Artifact, error) {
	return []run.Artifact{{Path: "report.md", Content: []byte("# Report\n"), SizeBytes: 9}}, nil
}
