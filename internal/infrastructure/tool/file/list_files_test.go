package file

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestListFilesReturnsSortedRelativePaths(t *testing.T) {
	root := newWorkspace(t)

	result := NewRunner(Config{}).listFiles(context.Background(), root)
	if result.IsError {
		t.Fatalf("listFiles() returned error result: %s", result.Content)
	}

	want := "files:\nREADME.md\ninternal/app.go"
	if result.Content != want {
		t.Fatalf("Content = %q, want %q", result.Content, want)
	}
}

func TestCollectFilesEnforcesMaxFileCount(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"one.txt", "two.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	_, err := NewRunner(Config{MaxFiles: 1}).collectFiles(context.Background(), root)
	if !errors.Is(err, errTooManyFiles) {
		t.Fatalf("collectFiles() error = %v, want %v", err, errTooManyFiles)
	}
}
