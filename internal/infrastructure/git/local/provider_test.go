package local

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestProviderImplementsGitProvider(_ *testing.T) {
	var _ outbound.GitProvider = (*Provider)(nil)
}

func TestNewProviderRejectsEmptyPath(t *testing.T) {
	_, err := NewProvider(Config{})
	if err == nil || !strings.Contains(err.Error(), "workspace path is required") {
		t.Fatalf("NewProvider() error = %v, want required path error", err)
	}
}

func TestNewProviderRejectsMissingPath(t *testing.T) {
	_, err := NewProvider(Config{WorkspacePath: filepath.Join(t.TempDir(), "missing")})
	if err == nil || !strings.Contains(err.Error(), "workspace path does not exist") {
		t.Fatalf("NewProvider() error = %v, want missing path error", err)
	}
}

func TestNewProviderRejectsFilePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "README.md")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err := NewProvider(Config{WorkspacePath: path})
	if err == nil || !strings.Contains(err.Error(), "workspace path must be a directory") {
		t.Fatalf("NewProvider() error = %v, want directory error", err)
	}
}

func TestProviderFetchReturnsAbsoluteResolvedWorkspacePath(t *testing.T) {
	root := t.TempDir()

	provider, err := NewProvider(Config{WorkspacePath: root})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}
	checkout, err := provider.Fetch(context.Background(), outbound.GitFetchRequest{
		RepositoryURL: "https://example.com/repo.git",
		Branch:        "main",
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	if checkout.Path != want {
		t.Fatalf("checkout path = %q, want %q", checkout.Path, want)
	}
	if !filepath.IsAbs(checkout.Path) {
		t.Fatalf("checkout path = %q, want absolute path", checkout.Path)
	}
}

func TestProviderResolvesSymlinkWorkspacePath(t *testing.T) {
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "workspace")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	provider, err := NewProvider(Config{WorkspacePath: link})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}
	checkout, err := provider.Fetch(context.Background(), outbound.GitFetchRequest{})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	want, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	if checkout.Path != want {
		t.Fatalf("checkout path = %q, want %q", checkout.Path, want)
	}
}
