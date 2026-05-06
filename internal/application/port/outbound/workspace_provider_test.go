package outbound

import (
	"context"
	"testing"

	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestWorkspaceProviderContract(t *testing.T) {
	var provider WorkspaceProvider = contractWorkspaceProvider{}
	var materializer WorkspaceMaterializer = contractWorkspaceProvider{}

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
	if workspace.StatePath == "" {
		t.Fatal("workspace state path is empty")
	}
	if len(workspace.Sources) != 1 || workspace.Sources[0].ID != "input_001" {
		t.Fatalf("workspace sources = %#v, want input_001", workspace.Sources)
	}
	staged, err := materializer.MaterializeWorkspaceSource(context.Background(), WorkspaceMaterializeRequest{
		Workspace: workspace,
		SourceID:  "input_001",
		Path:      "README.md",
	})
	if err != nil {
		t.Fatalf("MaterializeWorkspaceSource() error = %v", err)
	}
	if staged.Path != "README.md" || staged.SourceID != "input_001" {
		t.Fatalf("staged file = %#v, want README.md from input_001", staged)
	}
	stagedFiles, err := materializer.ListWorkspaceStagedFiles(context.Background(), workspace)
	if err != nil {
		t.Fatalf("ListWorkspaceStagedFiles() error = %v", err)
	}
	if len(stagedFiles) != 1 || stagedFiles[0].Path != "README.md" {
		t.Fatalf("staged files = %#v, want README.md", stagedFiles)
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
		StatePath: "/tmp/workspace-state",
		Sources: []WorkspaceSource{
			{ID: "input_001", Path: "README.md", SizeBytes: 7, Checksum: "sha256:test"},
		},
	}, nil
}

func (contractWorkspaceProvider) CleanupWorkspace(context.Context, Workspace) error {
	return nil
}

func (contractWorkspaceProvider) CollectArtifacts(context.Context, Workspace) ([]run.Artifact, error) {
	return []run.Artifact{{Path: "report.md", Content: []byte("# Report\n"), SizeBytes: 9}}, nil
}

func (contractWorkspaceProvider) MaterializeWorkspaceSource(
	context.Context,
	WorkspaceMaterializeRequest,
) (WorkspaceStagedFile, error) {
	return WorkspaceStagedFile{SourceID: "input_001", Path: "README.md", SizeBytes: 7, Checksum: "sha256:test"}, nil
}

func (contractWorkspaceProvider) ListWorkspaceStagedFiles(context.Context, Workspace) ([]WorkspaceStagedFile, error) {
	return []WorkspaceStagedFile{{SourceID: "input_001", Path: "README.md", SizeBytes: 7, Checksum: "sha256:test"}}, nil
}
