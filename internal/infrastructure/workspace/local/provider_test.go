package local

import (
	"context"
	"errors"
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

func TestResolveWorkspaceReturnsEmptyForNone(t *testing.T) {
	provider, err := NewProvider(Config{})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

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

func TestResolveWorkspaceRejectsMissingConfiguredPath(t *testing.T) {
	provider, err := NewProvider(Config{})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	_, err = provider.ResolveWorkspace(context.Background(), outbound.WorkspaceResolveRequest{
		Source: run.WorkspaceSource{Type: run.WorkspaceSourceConfigured},
	})
	if err == nil || !strings.Contains(err.Error(), "configured workspace path is required") {
		t.Fatalf("ResolveWorkspace() error = %v, want configured path error", err)
	}
}

func TestResolveWorkspaceRejectsMissingPath(t *testing.T) {
	provider, err := NewProvider(Config{ConfiguredPath: filepath.Join(t.TempDir(), "missing")})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	_, err = provider.ResolveWorkspace(context.Background(), outbound.WorkspaceResolveRequest{
		Source: run.WorkspaceSource{Type: run.WorkspaceSourceConfigured},
	})
	if err == nil || !strings.Contains(err.Error(), "workspace path does not exist") {
		t.Fatalf("ResolveWorkspace() error = %v, want missing path error", err)
	}
}

func TestResolveWorkspaceRejectsFilePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "README.md")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	provider, err := NewProvider(Config{ConfiguredPath: path})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	_, err = provider.ResolveWorkspace(context.Background(), outbound.WorkspaceResolveRequest{
		Source: run.WorkspaceSource{Type: run.WorkspaceSourceConfigured},
	})
	if err == nil || !strings.Contains(err.Error(), "workspace path must be a directory") {
		t.Fatalf("ResolveWorkspace() error = %v, want directory error", err)
	}
}

func TestResolveWorkspaceReturnsAbsoluteResolvedConfiguredPath(t *testing.T) {
	root := t.TempDir()
	provider, err := NewProvider(Config{ConfiguredPath: root})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	workspace, err := provider.ResolveWorkspace(context.Background(), outbound.WorkspaceResolveRequest{
		Source: run.WorkspaceSource{Type: run.WorkspaceSourceConfigured},
	})
	if err != nil {
		t.Fatalf("ResolveWorkspace() error = %v", err)
	}

	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	if workspace.Path != want {
		t.Fatalf("workspace path = %q, want %q", workspace.Path, want)
	}
	if !filepath.IsAbs(workspace.Path) {
		t.Fatalf("workspace path = %q, want absolute", workspace.Path)
	}
}

func TestResolveWorkspaceResolvesSymlinkConfiguredPath(t *testing.T) {
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "workspace")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	provider, err := NewProvider(Config{ConfiguredPath: link})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	workspace, err := provider.ResolveWorkspace(context.Background(), outbound.WorkspaceResolveRequest{
		Source: run.WorkspaceSource{Type: run.WorkspaceSourceConfigured},
	})
	if err != nil {
		t.Fatalf("ResolveWorkspace() error = %v", err)
	}

	want, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	if workspace.Path != want {
		t.Fatalf("workspace path = %q, want %q", workspace.Path, want)
	}
}

func TestResolveWorkspaceRejectsUnknownSource(t *testing.T) {
	provider, err := NewProvider(Config{})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	_, err = provider.ResolveWorkspace(context.Background(), outbound.WorkspaceResolveRequest{
		Source: run.WorkspaceSource{Type: "snapshot"},
	})
	if !errors.Is(err, run.ErrUnknownWorkspaceSource) {
		t.Fatalf("ResolveWorkspace() error = %v, want %v", err, run.ErrUnknownWorkspaceSource)
	}
}
