package file

import (
	"path/filepath"
	"testing"
)

func TestConfinedPathReturnsWorkspacePath(t *testing.T) {
	root := t.TempDir()

	target, err := confinedPath(root, "internal/app.go")
	if err != nil {
		t.Fatalf("confinedPath() error = %v", err)
	}

	want := filepath.Join(root, "internal", "app.go")
	if target != want {
		t.Fatalf("target = %q, want %q", target, want)
	}
}

func TestConfinedPathRejectsUnsafePaths(t *testing.T) {
	root := t.TempDir()

	tests := []string{
		"",
		"/absolute.txt",
		"../secret.txt",
		"./README.md",
		"internal//app.go",
		"internal\\app.go",
		"C:/workspace/file.txt",
	}
	for _, test := range tests {
		if _, err := confinedPath(root, test); err == nil {
			t.Fatalf("confinedPath(%q) error = nil, want error", test)
		}
	}
}
