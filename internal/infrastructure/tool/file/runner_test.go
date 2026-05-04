package file

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestRunnerDoesNotExposeToolsWithoutWorkspace(t *testing.T) {
	tools, err := NewRunner(Config{}).ListTools(context.Background(), outbound.ToolListRequest{})
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools) != 0 {
		t.Fatalf("len(tools) = %d, want 0", len(tools))
	}
}

func TestRunnerListsFiles(t *testing.T) {
	root := newWorkspace(t)

	result, err := NewRunner(Config{}).RunTool(context.Background(), outbound.ToolCall{
		Context: outbound.ToolContext{WorkspacePath: root},
		Name:    toolNameListFiles,
	})
	if err != nil {
		t.Fatalf("RunTool() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("RunTool() returned error result: %s", result.Content)
	}
	if !strings.Contains(result.Content, "README.md") || !strings.Contains(result.Content, "internal/app.go") {
		t.Fatalf("list_files result = %q, want file paths", result.Content)
	}
}

func TestRunnerReadsFile(t *testing.T) {
	root := newWorkspace(t)

	result, err := NewRunner(Config{}).RunTool(context.Background(), outbound.ToolCall{
		Context:   outbound.ToolContext{WorkspacePath: root},
		Name:      toolNameReadFile,
		Arguments: map[string]string{argumentPath: "README.md"},
	})
	if err != nil {
		t.Fatalf("RunTool() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("RunTool() returned error result: %s", result.Content)
	}
	if result.Content != "# Demo\n" {
		t.Fatalf("read_file content = %q, want # Demo", result.Content)
	}
}

func TestRunnerRejectsPathTraversal(t *testing.T) {
	root := newWorkspace(t)

	result, err := NewRunner(Config{}).RunTool(context.Background(), outbound.ToolCall{
		Context:   outbound.ToolContext{WorkspacePath: root},
		Name:      toolNameReadFile,
		Arguments: map[string]string{argumentPath: "../secret.txt"},
	})
	if err != nil {
		t.Fatalf("RunTool() error = %v", err)
	}
	if !result.IsError {
		t.Fatalf("RunTool() IsError = false, want true")
	}
}

func TestRunnerRejectsLargeFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "large.txt"), []byte("too large"), 0o600); err != nil {
		t.Fatalf("write large file: %v", err)
	}

	result, err := NewRunner(Config{MaxReadSizeBytes: 4}).RunTool(context.Background(), outbound.ToolCall{
		Context:   outbound.ToolContext{WorkspacePath: root},
		Name:      toolNameReadFile,
		Arguments: map[string]string{argumentPath: "large.txt"},
	})
	if err != nil {
		t.Fatalf("RunTool() error = %v", err)
	}
	if !result.IsError {
		t.Fatalf("RunTool() IsError = false, want true")
	}
	if !strings.Contains(result.Content, "maximum read size") {
		t.Fatalf("RunTool() content = %q, want size error", result.Content)
	}
}

func TestRunnerUnknownToolHandledSafely(t *testing.T) {
	result, err := NewRunner(Config{}).RunTool(context.Background(), outbound.ToolCall{
		Context: outbound.ToolContext{WorkspacePath: t.TempDir()},
		Name:    "unknown",
	})
	if err != nil {
		t.Fatalf("RunTool() error = %v", err)
	}
	if !result.IsError {
		t.Fatalf("RunTool() IsError = false, want true")
	}
	if !strings.Contains(result.Content, "unknown tool") {
		t.Fatalf("RunTool() content = %q, want unknown tool", result.Content)
	}
}

func newWorkspace(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# Demo\n"), 0o600); err != nil {
		t.Fatalf("write README: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "internal"), 0o700); err != nil {
		t.Fatalf("mkdir internal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "app.go"), []byte("package internal\n"), 0o600); err != nil {
		t.Fatalf("write app.go: %v", err)
	}

	return root
}
