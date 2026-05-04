package temp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestProviderImplementsWorkspaceProvider(_ *testing.T) {
	var _ outbound.WorkspaceProvider = (*Provider)(nil)
}

func TestPrepareWorkspaceWritesAttachments(t *testing.T) {
	provider := NewProvider(Config{BaseDir: t.TempDir()})

	workspace, err := provider.PrepareWorkspace(context.Background(), outbound.WorkspacePrepareRequest{
		RunID: "run_test",
		Attachments: []run.TaskAttachment{
			{Filename: "README.md", Content: []byte("# Demo\n"), SizeBytes: 7},
			{Filename: "internal/app.go", Content: []byte("package internal\n"), SizeBytes: 17},
		},
	})
	if err != nil {
		t.Fatalf("PrepareWorkspace() error = %v", err)
	}
	if workspace.Path == "" {
		t.Fatal("workspace path is empty")
	}

	content, err := os.ReadFile(filepath.Join(workspace.Path, "README.md"))
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	if string(content) != "# Demo\n" {
		t.Fatalf("README content = %q, want # Demo", content)
	}
	if _, err := os.Stat(filepath.Join(workspace.Path, "internal", "app.go")); err != nil {
		t.Fatalf("stat nested attachment: %v", err)
	}
}

func TestPrepareWorkspaceRejectsPathTraversal(t *testing.T) {
	baseDir := t.TempDir()
	provider := NewProvider(Config{BaseDir: baseDir})

	_, err := provider.PrepareWorkspace(context.Background(), outbound.WorkspacePrepareRequest{
		RunID: "run_test",
		Attachments: []run.TaskAttachment{
			{Filename: "../secret.txt", Content: []byte("secret"), SizeBytes: 6},
		},
	})
	if err == nil {
		t.Fatal("PrepareWorkspace() error = nil, want error")
	}

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		t.Fatalf("read base dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("base dir entries = %d, want 0 after cleanup", len(entries))
	}
}

func TestCleanupWorkspaceRemovesDirectory(t *testing.T) {
	provider := NewProvider(Config{BaseDir: t.TempDir()})
	workspace, err := provider.PrepareWorkspace(context.Background(), outbound.WorkspacePrepareRequest{
		RunID: "run_test",
		Attachments: []run.TaskAttachment{
			{Filename: "README.md", Content: []byte("# Demo\n"), SizeBytes: 7},
		},
	})
	if err != nil {
		t.Fatalf("PrepareWorkspace() error = %v", err)
	}

	if err := provider.CleanupWorkspace(context.Background(), workspace); err != nil {
		t.Fatalf("CleanupWorkspace() error = %v", err)
	}
	if _, err := os.Stat(workspace.Path); !os.IsNotExist(err) {
		t.Fatalf("workspace path still exists or stat error = %v", err)
	}
}

func TestPrepareWorkspaceReturnsEmptyForNoAttachments(t *testing.T) {
	baseDir := t.TempDir()
	provider := NewProvider(Config{BaseDir: baseDir})

	workspace, err := provider.PrepareWorkspace(context.Background(), outbound.WorkspacePrepareRequest{RunID: "run_test"})
	if err != nil {
		t.Fatalf("PrepareWorkspace() error = %v", err)
	}
	if workspace.Path != "" {
		t.Fatalf("workspace path = %q, want empty", workspace.Path)
	}

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		t.Fatalf("read base dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("base dir entries = %d, want 0", len(entries))
	}
}
