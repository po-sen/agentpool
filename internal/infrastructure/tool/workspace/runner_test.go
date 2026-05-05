package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestRunnerImplementsToolRunner(_ *testing.T) {
	var _ outbound.ToolRunner = (*Runner)(nil)
}

func TestRunnerAdvertisesOnlyWorkspace(t *testing.T) {
	runner := NewRunner(Config{})

	tools, err := runner.ListTools(context.Background(), outbound.ToolListRequest{
		Context: outbound.ToolContext{Workspace: testWorkspace(t)},
	})
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools) != 1 || tools[0].Name != toolNameWorkspace {
		t.Fatalf("tools = %#v, want workspace", tools)
	}
	if len(tools[0].Arguments) != 3 {
		t.Fatalf("arguments = %#v, want operation, area, path", tools[0].Arguments)
	}
}

func TestRunnerDoesNotAdvertiseWithoutWorkspace(t *testing.T) {
	runner := NewRunner(Config{})

	tools, err := runner.ListTools(context.Background(), outbound.ToolListRequest{})
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools) != 0 {
		t.Fatalf("tools = %#v, want none", tools)
	}
}

func TestRunnerListsInputFiles(t *testing.T) {
	workspace := testWorkspace(t)
	writeFile(t, workspace.InputPath, "README.md", "demo")
	writeFile(t, workspace.WorkPath, "scratch.txt", "scratch")

	result := runWorkspaceTool(t, NewRunner(Config{}), workspace, map[string]string{
		argumentOperation: operationList,
		argumentArea:      areaInput,
	})
	if result.IsError {
		t.Fatalf("result = %#v, want success", result)
	}
	assertContains(t, result.Content, "/workspace/input/README.md")
	assertContains(t, result.Content, "area: input")
	assertContains(t, result.Content, "relative_path: README.md")
	assertContains(t, result.Content, "size_bytes: 4")
	assertNotContains(t, result.Content, "/workspace/work/scratch.txt")
	assertNotContains(t, result.Content, workspace.InputPath)
}

func TestRunnerListsWorkFiles(t *testing.T) {
	workspace := testWorkspace(t)
	writeFile(t, workspace.InputPath, "README.md", "demo")
	writeFile(t, workspace.WorkPath, "scratch.txt", "scratch")

	result := runWorkspaceTool(t, NewRunner(Config{}), workspace, map[string]string{
		argumentOperation: operationList,
		argumentArea:      areaWork,
	})
	if result.IsError {
		t.Fatalf("result = %#v, want success", result)
	}
	assertContains(t, result.Content, "/workspace/work/scratch.txt")
	assertNotContains(t, result.Content, "/workspace/input/README.md")
	assertNotContains(t, result.Content, workspace.WorkPath)
}

func TestRunnerListsAllFiles(t *testing.T) {
	workspace := testWorkspace(t)
	writeFile(t, workspace.InputPath, "README.md", "demo")
	writeFile(t, workspace.WorkPath, "scratch.txt", "scratch")

	result := runWorkspaceTool(t, NewRunner(Config{}), workspace, map[string]string{
		argumentOperation: operationList,
	})
	if result.IsError {
		t.Fatalf("result = %#v, want success", result)
	}
	assertContains(t, result.Content, "/workspace/input/README.md")
	assertContains(t, result.Content, "/workspace/work/scratch.txt")
}

func TestRunnerStatsInputFile(t *testing.T) {
	workspace := testWorkspace(t)
	writeFile(t, workspace.InputPath, "README.md", "demo")

	result := runWorkspaceTool(t, NewRunner(Config{}), workspace, map[string]string{
		argumentOperation: operationStat,
		argumentArea:      areaInput,
		argumentPath:      "README.md",
	})
	if result.IsError {
		t.Fatalf("result = %#v, want success", result)
	}
	for _, want := range []string{
		"virtual_path: /workspace/input/README.md",
		"area: input",
		"relative_path: README.md",
		"size_bytes: 4",
		"type: file",
	} {
		assertContains(t, result.Content, want)
	}
	assertNotContains(t, result.Content, workspace.InputPath)
}

func TestRunnerStatsInputFileWithVirtualPath(t *testing.T) {
	workspace := testWorkspace(t)
	writeFile(t, workspace.InputPath, "README.md", "demo")

	result := runWorkspaceTool(t, NewRunner(Config{}), workspace, map[string]string{
		argumentOperation: operationStat,
		argumentPath:      "/workspace/input/README.md",
	})
	if result.IsError {
		t.Fatalf("result = %#v, want success", result)
	}
	assertContains(t, result.Content, "virtual_path: /workspace/input/README.md")
	assertContains(t, result.Content, "area: input")
	assertContains(t, result.Content, "relative_path: README.md")
	assertNotContains(t, result.Content, workspace.InputPath)
}

func TestRunnerStatsWorkFile(t *testing.T) {
	workspace := testWorkspace(t)
	writeFile(t, workspace.WorkPath, "scratch.txt", "scratch")

	result := runWorkspaceTool(t, NewRunner(Config{}), workspace, map[string]string{
		argumentOperation: operationStat,
		argumentArea:      areaWork,
		argumentPath:      "scratch.txt",
	})
	if result.IsError {
		t.Fatalf("result = %#v, want success", result)
	}
	assertContains(t, result.Content, "virtual_path: /workspace/work/scratch.txt")
	assertContains(t, result.Content, "area: work")
	assertContains(t, result.Content, "type: file")
	assertNotContains(t, result.Content, workspace.WorkPath)
}

func TestRunnerStatsWorkFileWithVirtualPath(t *testing.T) {
	workspace := testWorkspace(t)
	writeFile(t, workspace.WorkPath, "scratch.txt", "scratch")

	result := runWorkspaceTool(t, NewRunner(Config{}), workspace, map[string]string{
		argumentOperation: operationStat,
		argumentPath:      "/workspace/work/scratch.txt",
	})
	if result.IsError {
		t.Fatalf("result = %#v, want success", result)
	}
	assertContains(t, result.Content, "virtual_path: /workspace/work/scratch.txt")
	assertContains(t, result.Content, "area: work")
	assertContains(t, result.Content, "relative_path: scratch.txt")
	assertNotContains(t, result.Content, workspace.WorkPath)
}

func TestRunnerStatsDirectory(t *testing.T) {
	workspace := testWorkspace(t)

	result := runWorkspaceTool(t, NewRunner(Config{}), workspace, map[string]string{
		argumentOperation: operationStat,
		argumentArea:      areaInput,
		argumentPath:      ".",
	})
	if result.IsError {
		t.Fatalf("result = %#v, want success", result)
	}
	assertContains(t, result.Content, "virtual_path: /workspace/input")
	assertContains(t, result.Content, "type: directory")
}

func TestRunnerRejectsUnsafePaths(t *testing.T) {
	workspace := testWorkspace(t)
	runner := NewRunner(Config{})

	for _, unsafePath := range []string{"/tmp/file", "../secret", "a/../b", `a\b`, "C:/tmp/file", "./README.md"} {
		t.Run(unsafePath, func(t *testing.T) {
			result := runWorkspaceTool(t, runner, workspace, map[string]string{
				argumentOperation: operationStat,
				argumentArea:      areaInput,
				argumentPath:      unsafePath,
			})
			if !result.IsError || !strings.Contains(result.Content, "path is unsafe") {
				t.Fatalf("result = %#v, want unsafe path error", result)
			}
		})
	}
}

func TestRunnerRejectsConflictingAreaAndVirtualPath(t *testing.T) {
	workspace := testWorkspace(t)
	writeFile(t, workspace.InputPath, "README.md", "demo")

	result := runWorkspaceTool(t, NewRunner(Config{}), workspace, map[string]string{
		argumentOperation: operationStat,
		argumentArea:      areaWork,
		argumentPath:      "/workspace/input/README.md",
	})
	if !result.IsError || !strings.Contains(result.Content, "conflicts with virtual path area") {
		t.Fatalf("result = %#v, want conflict error", result)
	}
	assertNotContains(t, result.Content, workspace.RootPath)
}

func TestRunnerRejectsUnknownArea(t *testing.T) {
	result := runWorkspaceTool(t, NewRunner(Config{}), testWorkspace(t), map[string]string{
		argumentOperation: operationList,
		argumentArea:      "logs",
	})
	if !result.IsError || !strings.Contains(result.Content, "unknown workspace area") {
		t.Fatalf("result = %#v, want unknown area error", result)
	}
}

func TestRunnerRejectsUnknownOperation(t *testing.T) {
	result := runWorkspaceTool(t, NewRunner(Config{}), testWorkspace(t), map[string]string{
		argumentOperation: "read",
	})
	if !result.IsError || !strings.Contains(result.Content, "unknown workspace operation: read") {
		t.Fatalf("result = %#v, want unknown operation error", result)
	}
}

func TestRunnerRequiresOperation(t *testing.T) {
	result := runWorkspaceTool(t, NewRunner(Config{}), testWorkspace(t), nil)
	if !result.IsError || !strings.Contains(result.Content, "missing operation argument") {
		t.Fatalf("result = %#v, want missing operation error", result)
	}
}

func TestRunnerBoundsListOutput(t *testing.T) {
	workspace := testWorkspace(t)
	for i := 0; i < 3; i++ {
		writeFile(t, workspace.InputPath, fmt.Sprintf("file-%d.txt", i), "demo")
	}

	result := runWorkspaceTool(t, NewRunner(Config{MaxFiles: 2}), workspace, map[string]string{
		argumentOperation: operationList,
		argumentArea:      areaInput,
	})
	if !result.IsError || !strings.Contains(result.Content, "too many files") {
		t.Fatalf("result = %#v, want bounded output error", result)
	}
}

func TestRunnerListMissingInputPathReturnsSafeError(t *testing.T) {
	workspace := testWorkspace(t)

	result := runWorkspaceTool(t, NewRunner(Config{}), workspace, map[string]string{
		argumentOperation: operationList,
		argumentArea:      areaInput,
		argumentPath:      "missing",
	})

	assertMissingPathError(t, result, workspace)
}

func TestRunnerListMissingWorkPathReturnsSafeError(t *testing.T) {
	workspace := testWorkspace(t)

	result := runWorkspaceTool(t, NewRunner(Config{}), workspace, map[string]string{
		argumentOperation: operationList,
		argumentArea:      areaWork,
		argumentPath:      "missing",
	})

	assertMissingPathError(t, result, workspace)
}

func TestRunnerListMissingAllPathReturnsSafeError(t *testing.T) {
	workspace := testWorkspace(t)

	result := runWorkspaceTool(t, NewRunner(Config{}), workspace, map[string]string{
		argumentOperation: operationList,
		argumentArea:      areaAll,
		argumentPath:      "missing",
	})

	assertMissingPathError(t, result, workspace)
}

func TestRunnerListAllPathReturnsPartialMatches(t *testing.T) {
	workspace := testWorkspace(t)
	writeFile(t, workspace.WorkPath, "scratch.txt", "scratch")

	result := runWorkspaceTool(t, NewRunner(Config{}), workspace, map[string]string{
		argumentOperation: operationList,
		argumentArea:      areaAll,
		argumentPath:      "scratch.txt",
	})
	if result.IsError {
		t.Fatalf("result = %#v, want partial success", result)
	}
	assertContains(t, result.Content, "virtual_path: /workspace/work/scratch.txt")
	assertNotContains(t, result.Content, "path is not available")
	assertNotContains(t, result.Content, workspace.WorkPath)
}

func TestRunnerStatAllPathReturnsPartialMatches(t *testing.T) {
	workspace := testWorkspace(t)
	writeFile(t, workspace.WorkPath, "scratch.txt", "scratch")

	result := runWorkspaceTool(t, NewRunner(Config{}), workspace, map[string]string{
		argumentOperation: operationStat,
		argumentArea:      areaAll,
		argumentPath:      "scratch.txt",
	})
	if result.IsError {
		t.Fatalf("result = %#v, want partial success", result)
	}
	assertContains(t, result.Content, "virtual_path: /workspace/work/scratch.txt")
	assertNotContains(t, result.Content, "path is not available")
	assertNotContains(t, result.Content, workspace.WorkPath)
}

func TestRunnerRejectsUnknownToolName(t *testing.T) {
	result, err := NewRunner(Config{}).RunTool(context.Background(), outbound.ToolCall{
		Name:    "unknown_tool",
		Context: outbound.ToolContext{Workspace: testWorkspace(t)},
	})
	if err != nil {
		t.Fatalf("RunTool() error = %v", err)
	}
	if !result.IsError || result.Content != "unknown tool: unknown_tool" {
		t.Fatalf("result = %#v, want unknown tool error", result)
	}
}

func runWorkspaceTool(t *testing.T, runner *Runner, workspace outbound.Workspace, arguments map[string]string) outbound.ToolResult {
	t.Helper()

	result, err := runner.RunTool(context.Background(), outbound.ToolCall{
		Name:      toolNameWorkspace,
		Context:   outbound.ToolContext{Workspace: workspace},
		Arguments: arguments,
	})
	if err != nil {
		t.Fatalf("RunTool() error = %v", err)
	}

	return result
}

func testWorkspace(t *testing.T) outbound.Workspace {
	t.Helper()

	root := t.TempDir()
	inputPath := filepath.Join(root, "input")
	workPath := filepath.Join(root, "work")
	if err := os.Mkdir(inputPath, 0o700); err != nil {
		t.Fatalf("mkdir input: %v", err)
	}
	if err := os.Mkdir(workPath, 0o700); err != nil {
		t.Fatalf("mkdir work: %v", err)
	}

	return outbound.Workspace{
		RootPath:  root,
		InputPath: inputPath,
		WorkPath:  workPath,
	}
}

func writeFile(t *testing.T, root string, relativePath string, content string) {
	t.Helper()

	target := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatalf("mkdir parent: %v", err)
	}
	if err := os.WriteFile(target, []byte(content), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func assertContains(t *testing.T, got string, want string) {
	t.Helper()

	if !strings.Contains(got, want) {
		t.Fatalf("%q does not contain %q", got, want)
	}
}

func assertNotContains(t *testing.T, got string, want string) {
	t.Helper()

	if strings.Contains(got, want) {
		t.Fatalf("%q unexpectedly contains %q", got, want)
	}
}

func assertMissingPathError(t *testing.T, result outbound.ToolResult, workspace outbound.Workspace) {
	t.Helper()

	if !result.IsError || !strings.Contains(result.Content, "path is not available") {
		t.Fatalf("result = %#v, want missing path error", result)
	}
	assertNotContains(t, result.Content, workspace.RootPath)
	assertNotContains(t, result.Content, workspace.InputPath)
	assertNotContains(t, result.Content, workspace.WorkPath)
}
