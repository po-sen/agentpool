package temp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
	if workspace.RootPath == "" {
		t.Fatal("workspace root path is empty")
	}
	if workspace.InputPath != filepath.Join(workspace.RootPath, "input") {
		t.Fatalf("InputPath = %q, want root/input", workspace.InputPath)
	}
	if workspace.WorkPath != filepath.Join(workspace.RootPath, "work") {
		t.Fatalf("WorkPath = %q, want root/work", workspace.WorkPath)
	}
	if !workspace.HasFiles {
		t.Fatal("workspace HasFiles = false, want true")
	}

	content, err := os.ReadFile(filepath.Join(workspace.InputPath, "README.md"))
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	if string(content) != "# Demo\n" {
		t.Fatalf("README content = %q, want # Demo", content)
	}
	if _, err := os.Stat(filepath.Join(workspace.InputPath, "internal", "app.go")); err != nil {
		t.Fatalf("stat nested attachment: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace.RootPath, "README.md")); !os.IsNotExist(err) {
		t.Fatalf("root attachment stat error = %v, want not exist", err)
	}
	assertPerm(t, workspace.RootPath, workspaceDirPerm)
	assertPerm(t, workspace.InputPath, workspaceDirPerm)
	assertPerm(t, workspace.WorkPath, workspaceDirPerm)
	assertPerm(t, filepath.Join(workspace.InputPath, "README.md"), workspaceFilePerm)
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
	if _, err := os.Stat(workspace.RootPath); !os.IsNotExist(err) {
		t.Fatalf("workspace path still exists or stat error = %v", err)
	}
}

func TestCollectArtifactsReadsWorkFiles(t *testing.T) {
	provider := NewProvider(Config{BaseDir: t.TempDir()})
	workspace, err := provider.PrepareWorkspace(context.Background(), outbound.WorkspacePrepareRequest{RunID: "run_test"})
	if err != nil {
		t.Fatalf("PrepareWorkspace() error = %v", err)
	}
	defer func() {
		_ = provider.CleanupWorkspace(context.Background(), workspace)
	}()
	writeTempWorkspaceFile(t, filepath.Join(workspace.WorkPath, "report.md"), "# Report\n")
	writeTempWorkspaceFile(t, filepath.Join(workspace.WorkPath, "nested", "data.json"), "{}\n")
	if err := os.Symlink(filepath.Join(workspace.WorkPath, "report.md"), filepath.Join(workspace.WorkPath, "link.md")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	artifacts, err := provider.CollectArtifacts(context.Background(), workspace)
	if err != nil {
		t.Fatalf("CollectArtifacts() error = %v", err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("len(artifacts) = %d, want 2: %#v", len(artifacts), artifacts)
	}
	if artifacts[0].Path != "nested/data.json" || artifacts[1].Path != "report.md" {
		t.Fatalf("artifact paths = %#v, want sorted work-relative paths", artifacts)
	}
	if string(artifacts[1].Content) != "# Report\n" {
		t.Fatalf("artifact content = %q, want report", string(artifacts[1].Content))
	}
	if artifacts[1].SizeBytes != 9 {
		t.Fatalf("artifact size = %d, want 9", artifacts[1].SizeBytes)
	}
	if artifacts[1].MediaType != "text/markdown; charset=utf-8" {
		t.Fatalf("artifact media type = %q, want markdown", artifacts[1].MediaType)
	}
}

func TestCollectArtifactsSkipsOversizedFiles(t *testing.T) {
	provider := NewProvider(Config{BaseDir: t.TempDir()})
	workspace, err := provider.PrepareWorkspace(context.Background(), outbound.WorkspacePrepareRequest{RunID: "run_test"})
	if err != nil {
		t.Fatalf("PrepareWorkspace() error = %v", err)
	}
	defer func() {
		_ = provider.CleanupWorkspace(context.Background(), workspace)
	}()
	writeTempWorkspaceFile(t, filepath.Join(workspace.WorkPath, "large.txt"), strings.Repeat("a", int(run.MaxArtifactSizeBytes)+1))

	artifacts, err := provider.CollectArtifacts(context.Background(), workspace)
	if err != nil {
		t.Fatalf("CollectArtifacts() error = %v", err)
	}
	if len(artifacts) != 0 {
		t.Fatalf("len(artifacts) = %d, want 0", len(artifacts))
	}
}

func TestArtifactCollectorSkipsUnreadableFileWithoutDroppingPriorArtifacts(t *testing.T) {
	root := t.TempDir()
	goodPath := filepath.Join(root, "report.md")
	changedPath := filepath.Join(root, "changed.txt")
	writeTempWorkspaceFile(t, goodPath, "# Report\n")
	writeTempWorkspaceFile(t, changedPath, "changed\n")

	collector := artifactCollector{root: root}
	if err := collector.add(goodPath, 9); err != nil {
		t.Fatalf("add good artifact: %v", err)
	}
	if err := collector.add(changedPath, 100); err != nil {
		t.Fatalf("add changed artifact: %v", err)
	}
	if len(collector.artifacts) != 1 {
		t.Fatalf("len(artifacts) = %d, want 1: %#v", len(collector.artifacts), collector.artifacts)
	}
	if collector.artifacts[0].Path != "report.md" {
		t.Fatalf("artifact path = %q, want report.md", collector.artifacts[0].Path)
	}
}

func TestPrepareWorkspaceCreatesEmptyWorkspaceForNoAttachments(t *testing.T) {
	baseDir := t.TempDir()
	provider := NewProvider(Config{BaseDir: baseDir})

	workspace, err := provider.PrepareWorkspace(context.Background(), outbound.WorkspacePrepareRequest{RunID: "run_test"})
	if err != nil {
		t.Fatalf("PrepareWorkspace() error = %v", err)
	}
	if workspace.RootPath == "" {
		t.Fatal("workspace root path is empty")
	}
	if workspace.HasFiles {
		t.Fatal("workspace HasFiles = true, want false")
	}
	if _, err := os.Stat(workspace.RootPath); err != nil {
		t.Fatalf("stat empty workspace: %v", err)
	}
	if _, err := os.Stat(workspace.InputPath); err != nil {
		t.Fatalf("stat empty input workspace: %v", err)
	}
	if _, err := os.Stat(workspace.WorkPath); err != nil {
		t.Fatalf("stat empty work workspace: %v", err)
	}

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		t.Fatalf("read base dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("base dir entries = %d, want 1", len(entries))
	}
	if err := provider.CleanupWorkspace(context.Background(), workspace); err != nil {
		t.Fatalf("CleanupWorkspace() error = %v", err)
	}
	if _, err := os.Stat(workspace.RootPath); !os.IsNotExist(err) {
		t.Fatalf("workspace path still exists or stat error = %v", err)
	}
}

func assertPerm(t *testing.T, path string, want os.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s permissions = %v, want %v", path, got, want)
	}
}

func writeTempWorkspaceFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), workspaceDirPerm); err != nil {
		t.Fatalf("mkdir workspace file parent: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), workspaceFilePerm); err != nil {
		t.Fatalf("write workspace file: %v", err)
	}
}
