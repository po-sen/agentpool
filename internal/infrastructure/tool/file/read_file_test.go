package file

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadFileRejectsNonRegularFile(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "docs"), 0o700); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}

	result := NewRunner(Config{}).readFile(root, "docs")
	if !result.IsError {
		t.Fatal("IsError = false, want true")
	}
	if result.Content != "path is not a regular file" {
		t.Fatalf("Content = %q, want non-regular file error", result.Content)
	}
}

func TestReadFileRejectsNonUTF8Content(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "binary.txt"), []byte{0xff, 0xfe}, 0o600); err != nil {
		t.Fatalf("write binary file: %v", err)
	}

	result := NewRunner(Config{}).readFile(root, "binary.txt")
	if !result.IsError {
		t.Fatal("IsError = false, want true")
	}
	if result.Content != "file is not UTF-8 text" {
		t.Fatalf("Content = %q, want UTF-8 error", result.Content)
	}
}
