package diff

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestCollectorImplementsWorkspaceChangeCollector(_ *testing.T) {
	var _ outbound.WorkspaceChangeCollector = NewCollector()
}

func TestCollectWorkspaceChangesReturnsEmptyWithoutWorkspace(t *testing.T) {
	changes, err := NewCollector().CollectWorkspaceChanges(context.Background(), outbound.Workspace{})
	if err != nil {
		t.Fatalf("CollectWorkspaceChanges() error = %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("len(changes) = %d, want 0", len(changes))
	}
}

func TestCollectWorkspaceChangesDetectsAddedModifiedAndDeletedFiles(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "base")
	current := filepath.Join(root, "current")
	writeFile(t, filepath.Join(base, "deleted.txt"), "deleted")
	writeFile(t, filepath.Join(base, "modified.txt"), "old")
	writeFile(t, filepath.Join(base, "unchanged.txt"), "same")
	writeFile(t, filepath.Join(current, "added.txt"), "added")
	writeFile(t, filepath.Join(current, "modified.txt"), "new")
	writeFile(t, filepath.Join(current, "unchanged.txt"), "same")

	changes, err := NewCollector().CollectWorkspaceChanges(context.Background(), outbound.Workspace{
		Path:     current,
		BasePath: base,
	})
	if err != nil {
		t.Fatalf("CollectWorkspaceChanges() error = %v", err)
	}

	want := []outbound.WorkspaceChange{
		{Path: "added.txt", Status: outbound.WorkspaceChangeAdded},
		{Path: "deleted.txt", Status: outbound.WorkspaceChangeDeleted},
		{Path: "modified.txt", Status: outbound.WorkspaceChangeModified},
	}
	if !reflect.DeepEqual(changes, want) {
		t.Fatalf("changes = %#v, want %#v", changes, want)
	}
}

func TestCollectWorkspaceChangesUsesSlashSeparatedRelativePaths(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "base")
	current := filepath.Join(root, "current")
	writeFile(t, filepath.Join(base, "docs", "guide.md"), "old")
	writeFile(t, filepath.Join(current, "docs", "guide.md"), "new")

	changes, err := NewCollector().CollectWorkspaceChanges(context.Background(), outbound.Workspace{
		Path:     current,
		BasePath: base,
	})
	if err != nil {
		t.Fatalf("CollectWorkspaceChanges() error = %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("len(changes) = %d, want 1", len(changes))
	}
	if changes[0].Path != "docs/guide.md" {
		t.Fatalf("change path = %q, want docs/guide.md", changes[0].Path)
	}
}

func TestCollectWorkspaceChangesRejectsNonRegularFiles(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "base")
	current := filepath.Join(root, "current")
	if err := os.MkdirAll(base, 0o700); err != nil {
		t.Fatalf("mkdir base: %v", err)
	}
	if err := os.MkdirAll(current, 0o700); err != nil {
		t.Fatalf("mkdir current: %v", err)
	}
	if err := os.Symlink("/tmp/outside", filepath.Join(current, "link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	_, err := NewCollector().CollectWorkspaceChanges(context.Background(), outbound.Workspace{
		Path:     current,
		BasePath: base,
	})
	if err == nil {
		t.Fatal("CollectWorkspaceChanges() error = nil, want non-regular file error")
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
