package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestRunnerImplementsToolRunner(_ *testing.T) {
	var _ outbound.ToolRunner = NewRunner(Config{})
}

func TestRunnerListToolsReturnsEmptyWithoutWorkspacePath(t *testing.T) {
	runner := NewRunner(Config{})

	tools, err := runner.ListTools(context.Background(), outbound.ToolListRequest{})
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools) != 0 {
		t.Fatalf("len(tools) = %d, want 0", len(tools))
	}
}

func TestRunnerListToolsReturnsWorkspaceToolsWithWorkspacePath(t *testing.T) {
	runner := NewRunner(Config{})

	tools, err := runner.ListTools(context.Background(), outbound.ToolListRequest{
		Sandbox: outbound.Sandbox{WorkspacePath: t.TempDir()},
	})
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	assertToolNames(t, tools, []string{"list_files", "read_file"})
}

func TestRunnerListFilesListsFilesAndDirectoriesSorted(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "zeta"))
	mustWriteFile(t, filepath.Join(root, "alpha.txt"), "alpha")
	mustMkdir(t, filepath.Join(root, "beta"))

	result := runWorkspaceTool(t, root, "list_files", map[string]string{"path": "."})

	if result.IsError {
		t.Fatalf("IsError = true, content = %q", result.Content)
	}
	if result.Content != "alpha.txt\nbeta/\nzeta/" {
		t.Fatalf("Content = %q, want sorted entries", result.Content)
	}
}

func TestRunnerListFilesRejectsPathTraversal(t *testing.T) {
	root := t.TempDir()

	result := runWorkspaceTool(t, root, "list_files", map[string]string{"path": "../outside"})

	assertToolError(t, result, "path escapes workspace")
}

func TestRunnerListFilesRejectsAbsolutePath(t *testing.T) {
	root := t.TempDir()

	result := runWorkspaceTool(t, root, "list_files", map[string]string{"path": root})

	assertToolError(t, result, "path must be relative")
}

func TestRunnerListFilesRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	link := filepath.Join(root, "outside")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	result := runWorkspaceTool(t, root, "list_files", map[string]string{"path": "outside"})

	assertToolError(t, result, "path escapes workspace")
}

func TestRunnerListFilesRejectsEscapingSymlinkPrefixBeforeMissingTarget(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	link := filepath.Join(root, "outside")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	result := runWorkspaceTool(t, root, "list_files", map[string]string{"path": "outside/missing"})

	assertToolError(t, result, "path escapes workspace")
}

func TestRunnerReadFileReadsUTF8Text(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "README.md"), "hello\n")

	result := runWorkspaceTool(t, root, "read_file", map[string]string{"path": "README.md"})

	if result.IsError {
		t.Fatalf("IsError = true, content = %q", result.Content)
	}
	if result.Content != "hello\n" {
		t.Fatalf("Content = %q, want file content", result.Content)
	}
}

func TestRunnerReadFileRejectsMissingPath(t *testing.T) {
	root := t.TempDir()

	result := runWorkspaceTool(t, root, "read_file", map[string]string{})

	assertToolError(t, result, "missing path argument")
}

func TestRunnerReadFileRejectsDirectory(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "docs"))

	result := runWorkspaceTool(t, root, "read_file", map[string]string{"path": "docs"})

	assertToolError(t, result, "path is a directory")
}

func TestRunnerReadFileRejectsBinaryFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "data.bin"), []byte{0x00, 0x01, 0x02}, 0o600); err != nil {
		t.Fatalf("write binary file: %v", err)
	}

	result := runWorkspaceTool(t, root, "read_file", map[string]string{"path": "data.bin"})

	assertToolError(t, result, "file is binary")
}

func TestRunnerReadFileRejectsOversizedFile(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "large.txt"), "1234")
	runner := NewRunner(Config{MaxFileBytes: 3})

	result, err := runner.RunTool(context.Background(), outbound.ToolCall{
		Sandbox:   outbound.Sandbox{WorkspacePath: root},
		Name:      "read_file",
		Arguments: map[string]string{"path": "large.txt"},
	})
	if err != nil {
		t.Fatalf("RunTool() error = %v", err)
	}

	assertToolError(t, result, "file exceeds 3 bytes")
}

func TestRunnerReadFileRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	mustWriteFile(t, outside, "secret")

	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	result := runWorkspaceTool(t, root, "read_file", map[string]string{"path": "link.txt"})

	assertToolError(t, result, "path escapes workspace")
}

func TestRunnerReadFileRejectsEscapingSymlinkPrefixBeforeMissingTarget(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	link := filepath.Join(root, "outside")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	result := runWorkspaceTool(t, root, "read_file", map[string]string{"path": "outside/missing.txt"})

	assertToolError(t, result, "path escapes workspace")
}

func TestRunnerReadFileAllowsSymlinkWithinWorkspace(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "docs"))
	mustWriteFile(t, filepath.Join(root, "docs", "README.md"), "hello")

	link := filepath.Join(root, "link")
	if err := os.Symlink("docs", link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	result := runWorkspaceTool(t, root, "read_file", map[string]string{"path": "link/README.md"})

	if result.IsError {
		t.Fatalf("IsError = true, content = %q", result.Content)
	}
	if result.Content != "hello" {
		t.Fatalf("Content = %q, want hello", result.Content)
	}
}

func TestRunnerReadFileRejectsPathTraversal(t *testing.T) {
	root := t.TempDir()

	result := runWorkspaceTool(t, root, "read_file", map[string]string{"path": "../outside.txt"})

	assertToolError(t, result, "path escapes workspace")
}

func TestRunnerReadFileRejectsAbsolutePath(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "README.md"), "hello")

	result := runWorkspaceTool(t, root, "read_file", map[string]string{"path": filepath.Join(root, "README.md")})

	assertToolError(t, result, "path must be relative")
}

func TestRunnerReturnsToolErrorWhenWorkspacePathMissing(t *testing.T) {
	result := runWorkspaceTool(t, "", "list_files", map[string]string{})

	assertToolError(t, result, "workspace path is not available")
}

func TestRunnerUnknownToolReturnsToolError(t *testing.T) {
	root := t.TempDir()

	result := runWorkspaceTool(t, root, "missing", map[string]string{})

	assertToolError(t, result, "unknown tool: missing")
}

func runWorkspaceTool(t *testing.T, root string, name string, args map[string]string) outbound.ToolResult {
	t.Helper()

	runner := NewRunner(Config{})
	result, err := runner.RunTool(context.Background(), outbound.ToolCall{
		Sandbox:   outbound.Sandbox{WorkspacePath: root},
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("RunTool() error = %v", err)
	}

	return result
}

func assertToolNames(t *testing.T, tools []outbound.ToolDefinition, want []string) {
	t.Helper()

	if len(tools) != len(want) {
		t.Fatalf("len(tools) = %d, want %d", len(tools), len(want))
	}
	got := make([]string, 0, len(tools))
	for _, tool := range tools {
		got = append(got, tool.Name)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("tool names = %v, want %v", got, want)
	}
}

func assertToolError(t *testing.T, result outbound.ToolResult, content string) {
	t.Helper()

	if !result.IsError {
		t.Fatalf("IsError = false, want true; content = %q", result.Content)
	}
	if result.Content != content {
		t.Fatalf("Content = %q, want %q", result.Content, content)
	}
}

func mustWriteFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()

	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("make directory %s: %v", path, err)
	}
}
