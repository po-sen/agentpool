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

func TestProviderImplementsWorkspacePorts(_ *testing.T) {
	var _ outbound.WorkspaceProvider = (*Provider)(nil)
	var _ outbound.WorkspaceMaterializer = (*Provider)(nil)
}

func TestPrepareWorkspaceStoresAttachmentsAsSources(t *testing.T) {
	provider := NewProvider(Config{BaseDir: t.TempDir()})

	workspace, err := provider.PrepareWorkspace(context.Background(), outbound.WorkspacePrepareRequest{
		RunID: "run_test",
		Attachments: []run.TaskAttachment{
			{Filename: "README.md", MediaType: "text/markdown", Content: []byte("# Demo\n"), SizeBytes: 7},
			{Filename: "internal/app.go", Content: []byte("package internal\n"), SizeBytes: 17},
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
	if len(workspace.Sources) != 2 {
		t.Fatalf("len(Sources) = %d, want 2", len(workspace.Sources))
	}
	if workspace.Sources[0].ID != "input_001" || workspace.Sources[0].Path != "README.md" {
		t.Fatalf("first source = %#v, want README.md", workspace.Sources[0])
	}
	if workspace.Sources[0].MediaType != "text/markdown" {
		t.Fatalf("source media type = %q, want text/markdown", workspace.Sources[0].MediaType)
	}
	if workspace.Sources[0].Checksum == "" {
		t.Fatal("source checksum is empty")
	}
	if _, err := os.Stat(filepath.Join(workspace.RootPath, "README.md")); !os.IsNotExist(err) {
		t.Fatalf("workspace attachment stat error = %v, want not staged", err)
	}
	assertPerm(t, workspace.RootPath, workspaceDirPerm)
	assertPerm(t, workspace.StatePath, workspaceDirPerm)
}

func TestMaterializeWorkspaceSourceStagesFile(t *testing.T) {
	provider := NewProvider(Config{BaseDir: t.TempDir()})
	workspace := prepareWorkspaceWithReadme(t, provider)

	staged, err := provider.MaterializeWorkspaceSource(context.Background(), outbound.WorkspaceMaterializeRequest{
		Workspace: workspace,
		SourceID:  "input_001",
	})
	if err != nil {
		t.Fatalf("MaterializeWorkspaceSource() error = %v", err)
	}
	if staged.Path != "README.md" || staged.SourceID != "input_001" {
		t.Fatalf("staged = %#v, want README.md from input_001", staged)
	}
	content, err := os.ReadFile(filepath.Join(workspace.RootPath, "README.md"))
	if err != nil {
		t.Fatalf("read staged README: %v", err)
	}
	if string(content) != "# Demo\n" {
		t.Fatalf("README content = %q, want # Demo", string(content))
	}
	stagedFiles, err := provider.ListWorkspaceStagedFiles(context.Background(), workspace)
	if err != nil {
		t.Fatalf("ListWorkspaceStagedFiles() error = %v", err)
	}
	if len(stagedFiles) != 1 || stagedFiles[0].Path != "README.md" {
		t.Fatalf("staged files = %#v, want README.md", stagedFiles)
	}
}

func TestMaterializeWorkspaceSourceRejectsOverwriteWithoutRestore(t *testing.T) {
	provider := NewProvider(Config{BaseDir: t.TempDir()})
	workspace := prepareWorkspaceWithReadme(t, provider)
	writeTempWorkspaceFile(t, filepath.Join(workspace.RootPath, "README.md"), "changed\n")

	_, err := provider.MaterializeWorkspaceSource(context.Background(), outbound.WorkspaceMaterializeRequest{
		Workspace: workspace,
		SourceID:  "input_001",
	})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("MaterializeWorkspaceSource() error = %v, want already exists", err)
	}
}

func TestMaterializeWorkspaceSourceRestoresFile(t *testing.T) {
	provider := NewProvider(Config{BaseDir: t.TempDir()})
	workspace := prepareWorkspaceWithReadme(t, provider)
	writeTempWorkspaceFile(t, filepath.Join(workspace.RootPath, "README.md"), "changed\n")

	_, err := provider.MaterializeWorkspaceSource(context.Background(), outbound.WorkspaceMaterializeRequest{
		Workspace: workspace,
		SourceID:  "input_001",
		Overwrite: true,
	})
	if err != nil {
		t.Fatalf("MaterializeWorkspaceSource() error = %v", err)
	}
	content, err := os.ReadFile(filepath.Join(workspace.RootPath, "README.md"))
	if err != nil {
		t.Fatalf("read restored README: %v", err)
	}
	if string(content) != "# Demo\n" {
		t.Fatalf("README content = %q, want restored source", string(content))
	}
}

func TestMaterializeWorkspaceSourceRestoreReplacesLeafSymlink(t *testing.T) {
	provider := NewProvider(Config{BaseDir: t.TempDir()})
	workspace := prepareWorkspaceWithReadme(t, provider)
	outside := filepath.Join(t.TempDir(), "outside.txt")
	writeTempWorkspaceFile(t, outside, "outside\n")
	if err := os.Symlink(outside, filepath.Join(workspace.RootPath, "README.md")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	_, err := provider.MaterializeWorkspaceSource(context.Background(), outbound.WorkspaceMaterializeRequest{
		Workspace: workspace,
		SourceID:  "input_001",
		Overwrite: true,
	})
	if err != nil {
		t.Fatalf("MaterializeWorkspaceSource() error = %v", err)
	}
	outsideContent, err := os.ReadFile(outside)
	if err != nil {
		t.Fatalf("read outside: %v", err)
	}
	if string(outsideContent) != "outside\n" {
		t.Fatalf("outside content = %q, want untouched", string(outsideContent))
	}
	info, err := os.Lstat(filepath.Join(workspace.RootPath, "README.md"))
	if err != nil {
		t.Fatalf("lstat restored file: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("restored file is still a symlink")
	}
}

func TestMaterializeWorkspaceSourceRejectsSymlinkParent(t *testing.T) {
	provider := NewProvider(Config{BaseDir: t.TempDir()})
	workspace := prepareWorkspaceWithReadme(t, provider)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(workspace.RootPath, "docs")); err != nil {
		t.Fatalf("create parent symlink: %v", err)
	}

	_, err := provider.MaterializeWorkspaceSource(context.Background(), outbound.WorkspaceMaterializeRequest{
		Workspace: workspace,
		SourceID:  "input_001",
		Path:      "docs/README.md",
	})
	if err == nil || !strings.Contains(err.Error(), "workspace parent path is not a directory") {
		t.Fatalf("MaterializeWorkspaceSource() error = %v, want symlink parent rejection", err)
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
	workspace := prepareWorkspaceWithReadme(t, provider)
	runRoot := filepath.Dir(workspace.RootPath)

	if err := provider.CleanupWorkspace(context.Background(), workspace); err != nil {
		t.Fatalf("CleanupWorkspace() error = %v", err)
	}
	if _, err := os.Stat(runRoot); !os.IsNotExist(err) {
		t.Fatalf("workspace path still exists or stat error = %v", err)
	}
}

func TestCollectArtifactsReadsNewAndChangedWorkspaceFiles(t *testing.T) {
	provider := NewProvider(Config{BaseDir: t.TempDir()})
	workspace := prepareWorkspaceWithReadme(t, provider)
	if _, err := provider.MaterializeWorkspaceSource(context.Background(), outbound.WorkspaceMaterializeRequest{
		Workspace: workspace,
		SourceID:  "input_001",
	}); err != nil {
		t.Fatalf("stage README: %v", err)
	}
	writeTempWorkspaceFile(t, filepath.Join(workspace.RootPath, "report.md"), "# Report\n")
	writeTempWorkspaceFile(t, filepath.Join(workspace.RootPath, "README.md"), "# Changed\n")
	writeTempWorkspaceFile(t, filepath.Join(workspace.RootPath, "nested", "data.json"), "{}\n")
	if err := os.Symlink(filepath.Join(workspace.RootPath, "report.md"), filepath.Join(workspace.RootPath, "link.md")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	artifacts, err := provider.CollectArtifacts(context.Background(), workspace)
	if err != nil {
		t.Fatalf("CollectArtifacts() error = %v", err)
	}
	if len(artifacts) != 3 {
		t.Fatalf("len(artifacts) = %d, want 3: %#v", len(artifacts), artifacts)
	}
	if artifacts[0].Path != "README.md" || artifacts[1].Path != "nested/data.json" || artifacts[2].Path != "report.md" {
		t.Fatalf("artifact paths = %#v, want changed and new workspace files", artifacts)
	}
	if string(artifacts[2].Content) != "# Report\n" {
		t.Fatalf("artifact content = %q, want report", string(artifacts[2].Content))
	}
	if artifacts[2].MediaType != "text/markdown; charset=utf-8" {
		t.Fatalf("artifact media type = %q, want markdown", artifacts[2].MediaType)
	}
}

func TestCollectArtifactsSkipsUnchangedStagedSources(t *testing.T) {
	provider := NewProvider(Config{BaseDir: t.TempDir()})
	workspace := prepareWorkspaceWithReadme(t, provider)
	if _, err := provider.MaterializeWorkspaceSource(context.Background(), outbound.WorkspaceMaterializeRequest{
		Workspace: workspace,
		SourceID:  "input_001",
	}); err != nil {
		t.Fatalf("stage README: %v", err)
	}

	artifacts, err := provider.CollectArtifacts(context.Background(), workspace)
	if err != nil {
		t.Fatalf("CollectArtifacts() error = %v", err)
	}
	if len(artifacts) != 0 {
		t.Fatalf("len(artifacts) = %d, want 0", len(artifacts))
	}
}

func TestCollectArtifactsSkipsOversizedFiles(t *testing.T) {
	provider := NewProvider(Config{BaseDir: t.TempDir()})
	workspace, err := provider.PrepareWorkspace(context.Background(), outbound.WorkspacePrepareRequest{RunID: "run_test"})
	if err != nil {
		t.Fatalf("PrepareWorkspace() error = %v", err)
	}
	writeTempWorkspaceFile(t, filepath.Join(workspace.RootPath, "large.txt"), strings.Repeat("a", int(run.MaxArtifactSizeBytes)+1))

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
	if workspace.RootPath == "" || workspace.StatePath == "" {
		t.Fatalf("workspace paths = %#v, want root and state", workspace)
	}
	if len(workspace.Sources) != 0 {
		t.Fatalf("sources = %#v, want none", workspace.Sources)
	}
	if _, err := os.Stat(workspace.RootPath); err != nil {
		t.Fatalf("stat empty workspace: %v", err)
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
}

func prepareWorkspaceWithReadme(t *testing.T, provider *Provider) outbound.Workspace {
	t.Helper()

	workspace, err := provider.PrepareWorkspace(context.Background(), outbound.WorkspacePrepareRequest{
		RunID: "run_test",
		Attachments: []run.TaskAttachment{
			{Filename: "README.md", MediaType: "text/markdown", Content: []byte("# Demo\n"), SizeBytes: 7},
		},
	})
	if err != nil {
		t.Fatalf("PrepareWorkspace() error = %v", err)
	}

	return workspace
}

func writeTempWorkspaceFile(t *testing.T, pathValue string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(pathValue), workspaceDirPerm); err != nil {
		t.Fatalf("mkdir parent: %v", err)
	}
	if err := os.WriteFile(pathValue, []byte(content), workspaceFilePerm); err != nil {
		t.Fatalf("write workspace file: %v", err)
	}
}

func assertPerm(t *testing.T, pathValue string, want os.FileMode) {
	t.Helper()

	info, err := os.Stat(pathValue)
	if err != nil {
		t.Fatalf("stat %s: %v", pathValue, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode %s = %v, want %v", pathValue, got, want)
	}
}
