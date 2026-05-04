package snapshot

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestProviderImplementsWorkspaceProvider(t *testing.T) {
	provider, err := NewProvider(fakeSnapshotStore{}, Config{})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	var _ outbound.WorkspaceProvider = provider
}

func TestNewProviderRejectsNilStore(t *testing.T) {
	_, err := NewProvider(nil, Config{})
	if err == nil || !strings.Contains(err.Error(), "snapshot store is required") {
		t.Fatalf("NewProvider() error = %v, want store required", err)
	}
}

func TestResolveWorkspaceReturnsEmptyForNone(t *testing.T) {
	provider := newProvider(t, fakeSnapshotStore{})

	workspace, err := provider.ResolveWorkspace(context.Background(), outbound.WorkspaceResolveRequest{
		Source: run.WorkspaceSource{Type: run.WorkspaceSourceNone},
	})
	if err != nil {
		t.Fatalf("ResolveWorkspace() error = %v", err)
	}
	if workspace.Path != "" {
		t.Fatalf("workspace path = %q, want empty", workspace.Path)
	}
}

func TestResolveWorkspaceMaterializesSnapshot(t *testing.T) {
	archive := zipArchive(t, map[string]string{
		"README.md":     "hello",
		"docs/guide.md": "guide",
	})
	provider := newProvider(t, fakeSnapshotStore{content: archive})

	workspace, err := provider.ResolveWorkspace(context.Background(), outbound.WorkspaceResolveRequest{
		Source: run.WorkspaceSource{Type: run.WorkspaceSourceSnapshot, SnapshotID: "wsnap_test"},
	})
	if err != nil {
		t.Fatalf("ResolveWorkspace() error = %v", err)
	}
	if workspace.Path == "" {
		t.Fatal("workspace path is empty")
	}
	assertFileContent(t, filepath.Join(workspace.Path, "README.md"), "hello")
	assertFileContent(t, filepath.Join(workspace.Path, "docs", "guide.md"), "guide")

	if err := provider.CleanupWorkspace(context.Background(), workspace); err != nil {
		t.Fatalf("CleanupWorkspace() error = %v", err)
	}
	if _, err := os.Stat(workspace.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("workspace still exists after cleanup: %v", err)
	}
}

func TestResolveWorkspacePropagatesSnapshotNotFound(t *testing.T) {
	provider := newProvider(t, fakeSnapshotStore{err: outbound.ErrSnapshotNotFound})

	_, err := provider.ResolveWorkspace(context.Background(), outbound.WorkspaceResolveRequest{
		Source: run.WorkspaceSource{Type: run.WorkspaceSourceSnapshot, SnapshotID: "missing"},
	})
	if !errors.Is(err, outbound.ErrSnapshotNotFound) {
		t.Fatalf("ResolveWorkspace() error = %v, want %v", err, outbound.ErrSnapshotNotFound)
	}
}

func TestResolveWorkspaceRejectsPathTraversal(t *testing.T) {
	provider := newProvider(t, fakeSnapshotStore{content: zipArchive(t, map[string]string{
		"../secret.txt": "secret",
	})})

	_, err := provider.ResolveWorkspace(context.Background(), outbound.WorkspaceResolveRequest{
		Source: run.WorkspaceSource{Type: run.WorkspaceSourceSnapshot, SnapshotID: "wsnap_test"},
	})
	if err == nil || !strings.Contains(err.Error(), "escapes workspace") {
		t.Fatalf("ResolveWorkspace() error = %v, want path escape error", err)
	}
}

func TestResolveWorkspaceRejectsSymlink(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	header := &zip.FileHeader{Name: "link"}
	header.SetMode(os.ModeSymlink | 0o777)
	file, err := writer.CreateHeader(header)
	if err != nil {
		t.Fatalf("create symlink header: %v", err)
	}
	if _, err := file.Write([]byte("target")); err != nil {
		t.Fatalf("write symlink content: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	provider := newProvider(t, fakeSnapshotStore{content: buffer.Bytes()})

	_, err = provider.ResolveWorkspace(context.Background(), outbound.WorkspaceResolveRequest{
		Source: run.WorkspaceSource{Type: run.WorkspaceSourceSnapshot, SnapshotID: "wsnap_test"},
	})
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("ResolveWorkspace() error = %v, want regular file error", err)
	}
}

func TestResolveWorkspaceRejectsTooManyFiles(t *testing.T) {
	provider := newProvider(t, fakeSnapshotStore{content: zipArchive(t, map[string]string{
		"one.txt": "1",
		"two.txt": "2",
	})}, Config{MaxFiles: 1})

	_, err := provider.ResolveWorkspace(context.Background(), outbound.WorkspaceResolveRequest{
		Source: run.WorkspaceSource{Type: run.WorkspaceSourceSnapshot, SnapshotID: "wsnap_test"},
	})
	if err == nil || !strings.Contains(err.Error(), "exceeds 1 files") {
		t.Fatalf("ResolveWorkspace() error = %v, want file count error", err)
	}
}

func TestResolveWorkspaceRejectsOversizedFile(t *testing.T) {
	provider := newProvider(t, fakeSnapshotStore{content: zipArchive(t, map[string]string{
		"large.txt": "1234",
	})}, Config{MaxFileBytes: 3})

	_, err := provider.ResolveWorkspace(context.Background(), outbound.WorkspaceResolveRequest{
		Source: run.WorkspaceSource{Type: run.WorkspaceSourceSnapshot, SnapshotID: "wsnap_test"},
	})
	if err == nil || !strings.Contains(err.Error(), "exceeds 3 bytes") {
		t.Fatalf("ResolveWorkspace() error = %v, want size error", err)
	}
}

func TestResolveWorkspaceRejectsUnknownSource(t *testing.T) {
	provider := newProvider(t, fakeSnapshotStore{})

	_, err := provider.ResolveWorkspace(context.Background(), outbound.WorkspaceResolveRequest{
		Source: run.WorkspaceSource{Type: "mounted"},
	})
	if !errors.Is(err, run.ErrUnknownWorkspaceSource) {
		t.Fatalf("ResolveWorkspace() error = %v, want %v", err, run.ErrUnknownWorkspaceSource)
	}
}

func newProvider(t *testing.T, store fakeSnapshotStore, options ...Config) *Provider {
	t.Helper()

	cfg := Config{TempDir: t.TempDir()}
	if len(options) > 0 {
		cfg.MaxFiles = options[0].MaxFiles
		cfg.MaxFileBytes = options[0].MaxFileBytes
		cfg.MaxTotalBytes = options[0].MaxTotalBytes
	}
	provider, err := NewProvider(store, cfg)
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	return provider
}

func zipArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range files {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create zip file %s: %v", name, err)
		}
		if _, err := file.Write([]byte(content)); err != nil {
			t.Fatalf("write zip file %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	return buffer.Bytes()
}

func assertFileContent(t *testing.T, path string, want string) {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file %s: %v", path, err)
	}
	if string(content) != want {
		t.Fatalf("content = %q, want %q", content, want)
	}
}

type fakeSnapshotStore struct {
	content []byte
	err     error
}

func (s fakeSnapshotStore) OpenSnapshot(context.Context, outbound.SnapshotOpenRequest) (io.ReadCloser, error) {
	if s.err != nil {
		return nil, s.err
	}

	return io.NopCloser(bytes.NewReader(s.content)), nil
}
