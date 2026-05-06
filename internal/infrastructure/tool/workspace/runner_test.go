package workspace

import (
	"context"
	"errors"
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

func TestNewRunnerRequiresMaterializer(t *testing.T) {
	_, err := NewRunner(nil, Config{})
	if err == nil || !strings.Contains(err.Error(), "workspace materializer is required") {
		t.Fatalf("NewRunner() error = %v, want materializer error", err)
	}
}

func TestRunnerAdvertisesWorkspaceControlPlane(t *testing.T) {
	runner := newTestRunner(t, nil)

	tools, err := runner.ListTools(context.Background(), outbound.ToolListRequest{
		Context: outbound.ToolContext{Workspace: testWorkspace(t)},
	})
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools) != 1 || tools[0].Name != toolNameWorkspace {
		t.Fatalf("tools = %#v, want workspace", tools)
	}
	if tools[0].Description != "Manages authorized input sources and staged files for the mutable /workspace." {
		t.Fatalf("description = %q", tools[0].Description)
	}
	if len(tools[0].Arguments) != 4 {
		t.Fatalf("arguments = %#v, want operation, source_id, source_ids, path", tools[0].Arguments)
	}
}

func TestRunnerDoesNotAdvertiseWithoutWorkspace(t *testing.T) {
	runner := newTestRunner(t, nil)

	tools, err := runner.ListTools(context.Background(), outbound.ToolListRequest{})
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools) != 0 {
		t.Fatalf("tools = %#v, want none", tools)
	}
}

func TestRunnerListsSources(t *testing.T) {
	materializer := &fakeMaterializer{
		staged: []outbound.WorkspaceStagedFile{{SourceID: "input_001", Path: "README.md", SizeBytes: 4}},
	}
	result := runWorkspaceTool(t, newTestRunner(t, materializer), testWorkspace(t), map[string]string{
		argumentOperation: operationListSources,
	})
	if result.IsError {
		t.Fatalf("result = %#v, want success", result)
	}
	for _, want := range []string{
		"sources:",
		"source_id: input_001",
		"path: README.md",
		"virtual_path: /workspace/README.md",
		"media_type: text/markdown",
		"size_bytes: 4",
		"checksum: sha256:readme",
		"staged: true",
		"staged_paths: /workspace/README.md",
	} {
		assertContains(t, result.Content, want)
	}
}

func TestRunnerStagesSource(t *testing.T) {
	materializer := &fakeMaterializer{}
	workspace := testWorkspace(t)

	result := runWorkspaceTool(t, newTestRunner(t, materializer), workspace, map[string]string{
		argumentOperation: operationStage,
		argumentSourceID:  "input_001",
	})
	if result.IsError {
		t.Fatalf("result = %#v, want success", result)
	}
	if materializer.requests[0].SourceID != "input_001" || materializer.requests[0].Path != "README.md" {
		t.Fatalf("materialize request = %#v, want README.md source", materializer.requests[0])
	}
	if materializer.requests[0].Overwrite {
		t.Fatal("stage Overwrite = true, want false")
	}
	assertContains(t, result.Content, "staged:")
	assertContains(t, result.Content, "virtual_path: /workspace/README.md")
}

func TestRunnerStagesSourceAtCustomPath(t *testing.T) {
	materializer := &fakeMaterializer{}
	workspace := testWorkspace(t)

	result := runWorkspaceTool(t, newTestRunner(t, materializer), workspace, map[string]string{
		argumentOperation: operationStage,
		argumentSourceID:  "input_001",
		argumentPath:      "/workspace/docs/README.md",
	})
	if result.IsError {
		t.Fatalf("result = %#v, want success", result)
	}
	if materializer.requests[0].Path != "docs/README.md" {
		t.Fatalf("materialize path = %q, want docs/README.md", materializer.requests[0].Path)
	}
}

func TestRunnerStagesManySources(t *testing.T) {
	materializer := &fakeMaterializer{}
	workspace := testWorkspaceWithSources(t, []outbound.WorkspaceSource{
		{ID: "input_001", Path: "README.md", SizeBytes: 4, Checksum: "sha256:readme"},
		{ID: "input_002", Path: "docs/spec.md", SizeBytes: 8, Checksum: "sha256:spec"},
	})

	result := runWorkspaceTool(t, newTestRunner(t, materializer), workspace, map[string]string{
		argumentOperation: operationStageMany,
		argumentSourceIDs: "input_001,input_002",
	})
	if result.IsError {
		t.Fatalf("result = %#v, want success", result)
	}
	if len(materializer.requests) != 2 {
		t.Fatalf("materialize request count = %d, want 2", len(materializer.requests))
	}
	for index, want := range []struct {
		sourceID string
		path     string
	}{
		{sourceID: "input_001", path: "README.md"},
		{sourceID: "input_002", path: "docs/spec.md"},
	} {
		if materializer.requests[index].SourceID != want.sourceID ||
			materializer.requests[index].Path != want.path ||
			materializer.requests[index].Overwrite {
			t.Fatalf("materialize request %d = %#v, want source %s path %s no overwrite",
				index,
				materializer.requests[index],
				want.sourceID,
				want.path,
			)
		}
	}
	assertContains(t, result.Content, "staged:")
	assertContains(t, result.Content, "source_id: input_001")
	assertContains(t, result.Content, "virtual_path: /workspace/README.md")
	assertContains(t, result.Content, "source_id: input_002")
	assertContains(t, result.Content, "virtual_path: /workspace/docs/spec.md")
}

func TestRunnerStageManyRejectsUnknownSourceBeforeMaterializing(t *testing.T) {
	materializer := &fakeMaterializer{}

	result := runWorkspaceTool(t, newTestRunner(t, materializer), testWorkspace(t), map[string]string{
		argumentOperation: operationStageMany,
		argumentSourceIDs: "input_001,input_404",
	})
	if !result.IsError || !strings.Contains(result.Content, "workspace source is not available: input_404") {
		t.Fatalf("result = %#v, want unknown source error", result)
	}
	if len(materializer.requests) != 0 {
		t.Fatalf("materialize request count = %d, want 0", len(materializer.requests))
	}
}

func TestRunnerStageManyReportsPartialFailure(t *testing.T) {
	materializer := &fakeMaterializer{errorsBySource: map[string]error{
		"input_002": errors.New("workspace path already exists; use restore to overwrite"),
	}}
	workspace := testWorkspaceWithSources(t, []outbound.WorkspaceSource{
		{ID: "input_001", Path: "README.md", SizeBytes: 4, Checksum: "sha256:readme"},
		{ID: "input_002", Path: "docs/spec.md", SizeBytes: 8, Checksum: "sha256:spec"},
	})

	result := runWorkspaceTool(t, newTestRunner(t, materializer), workspace, map[string]string{
		argumentOperation: operationStageMany,
		argumentSourceIDs: "input_001 input_002 input_001",
	})
	if !result.IsError {
		t.Fatalf("result = %#v, want partial failure", result)
	}
	if len(materializer.requests) != 2 {
		t.Fatalf("materialize request count = %d, want deduped count 2", len(materializer.requests))
	}
	assertContains(t, result.Content, "source_id: input_001")
	assertContains(t, result.Content, "failed:")
	assertContains(t, result.Content, "source_id: input_002")
	assertContains(t, result.Content, "workspace path already exists; use restore to overwrite")
}

func TestRunnerRestoresByPath(t *testing.T) {
	materializer := &fakeMaterializer{
		staged: []outbound.WorkspaceStagedFile{{SourceID: "input_001", Path: "README.md", SizeBytes: 4}},
	}

	result := runWorkspaceTool(t, newTestRunner(t, materializer), testWorkspace(t), map[string]string{
		argumentOperation: operationRestore,
		argumentPath:      "README.md",
	})
	if result.IsError {
		t.Fatalf("result = %#v, want success", result)
	}
	if materializer.requests[0].SourceID != "input_001" || materializer.requests[0].Path != "README.md" {
		t.Fatalf("restore request = %#v, want README.md from input_001", materializer.requests[0])
	}
	if !materializer.requests[0].Overwrite {
		t.Fatal("restore Overwrite = false, want true")
	}
	assertContains(t, result.Content, "restored:")
}

func TestRunnerListsWorkspaceFiles(t *testing.T) {
	workspace := testWorkspace(t)
	writeFile(t, workspace.RootPath, "README.md", "demo")
	writeFile(t, workspace.RootPath, "scratch.txt", "scratch")
	materializer := &fakeMaterializer{
		staged: []outbound.WorkspaceStagedFile{{SourceID: "input_001", Path: "README.md", SizeBytes: 4}},
	}

	result := runWorkspaceTool(t, newTestRunner(t, materializer), workspace, map[string]string{
		argumentOperation: operationList,
	})
	if result.IsError {
		t.Fatalf("result = %#v, want success", result)
	}
	assertContains(t, result.Content, "/workspace/README.md")
	assertContains(t, result.Content, "source_id: input_001")
	assertContains(t, result.Content, "/workspace/scratch.txt")
	assertNotContains(t, result.Content, workspace.RootPath)
}

func TestRunnerRejectsUnsafePaths(t *testing.T) {
	workspace := testWorkspace(t)
	runner := newTestRunner(t, nil)

	for _, unsafePath := range []string{"/tmp/file", "../secret", "a/../b", `a\b`, "C:/tmp/file", "./README.md"} {
		t.Run(unsafePath, func(t *testing.T) {
			result := runWorkspaceTool(t, runner, workspace, map[string]string{
				argumentOperation: operationList,
				argumentPath:      unsafePath,
			})
			if !result.IsError || !strings.Contains(result.Content, "path is unsafe") {
				t.Fatalf("result = %#v, want unsafe path error", result)
			}
		})
	}
}

func TestRunnerRejectsUnknownOperation(t *testing.T) {
	result := runWorkspaceTool(t, newTestRunner(t, nil), testWorkspace(t), map[string]string{
		argumentOperation: "read",
	})
	if !result.IsError || !strings.Contains(result.Content, "unknown workspace operation: read") {
		t.Fatalf("result = %#v, want unknown operation error", result)
	}
}

func TestRunnerRequiresOperation(t *testing.T) {
	result := runWorkspaceTool(t, newTestRunner(t, nil), testWorkspace(t), nil)
	if !result.IsError || !strings.Contains(result.Content, "missing operation argument") {
		t.Fatalf("result = %#v, want missing operation error", result)
	}
}

func TestRunnerBoundsListOutput(t *testing.T) {
	workspace := testWorkspace(t)
	for i := 0; i < 3; i++ {
		writeFile(t, workspace.RootPath, fmt.Sprintf("file-%d.txt", i), "demo")
	}

	result := runWorkspaceTool(t, newTestRunnerWithConfig(t, nil, Config{MaxFiles: 2}), workspace, map[string]string{
		argumentOperation: operationList,
	})
	if !result.IsError || !strings.Contains(result.Content, "too many files") {
		t.Fatalf("result = %#v, want bounded output error", result)
	}
}

func TestRunnerDefaultListLimitMatchesPOCUploadLimit(t *testing.T) {
	runner := newTestRunner(t, nil)
	if runner.maxFiles != 500 {
		t.Fatalf("default max files = %d, want 500", runner.maxFiles)
	}
}

func TestRunnerListMissingPathReturnsSafeError(t *testing.T) {
	workspace := testWorkspace(t)

	result := runWorkspaceTool(t, newTestRunner(t, nil), workspace, map[string]string{
		argumentOperation: operationList,
		argumentPath:      "missing",
	})

	if !result.IsError || !strings.Contains(result.Content, "path is not available") {
		t.Fatalf("result = %#v, want missing path error", result)
	}
	assertNotContains(t, result.Content, workspace.RootPath)
}

func TestRunnerRejectsUnknownToolName(t *testing.T) {
	runner := newTestRunner(t, nil)
	result, err := runner.RunTool(context.Background(), outbound.ToolCall{
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

func newTestRunner(t *testing.T, materializer *fakeMaterializer) *Runner {
	t.Helper()

	return newTestRunnerWithConfig(t, materializer, Config{})
}

func newTestRunnerWithConfig(t *testing.T, materializer *fakeMaterializer, cfg Config) *Runner {
	t.Helper()
	if materializer == nil {
		materializer = &fakeMaterializer{}
	}
	runner, err := NewRunner(materializer, cfg)
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	return runner
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

	return testWorkspaceWithSources(t, []outbound.WorkspaceSource{
		{
			ID:        "input_001",
			Path:      "README.md",
			MediaType: "text/markdown",
			SizeBytes: 4,
			Checksum:  "sha256:readme",
		},
	})
}

func testWorkspaceWithSources(t *testing.T, sources []outbound.WorkspaceSource) outbound.Workspace {
	t.Helper()

	return outbound.Workspace{
		RootPath:  t.TempDir(),
		StatePath: t.TempDir(),
		Sources:   sources,
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

type fakeMaterializer struct {
	requests       []outbound.WorkspaceMaterializeRequest
	staged         []outbound.WorkspaceStagedFile
	err            error
	errorsBySource map[string]error
}

func (m *fakeMaterializer) MaterializeWorkspaceSource(
	_ context.Context,
	request outbound.WorkspaceMaterializeRequest,
) (outbound.WorkspaceStagedFile, error) {
	m.requests = append(m.requests, request)
	if m.err != nil {
		return outbound.WorkspaceStagedFile{}, m.err
	}
	if err := m.errorsBySource[request.SourceID]; err != nil {
		return outbound.WorkspaceStagedFile{}, err
	}

	return outbound.WorkspaceStagedFile{
		SourceID:  request.SourceID,
		Path:      request.Path,
		SizeBytes: 4,
		Checksum:  "sha256:readme",
	}, nil
}

func (m *fakeMaterializer) ListWorkspaceStagedFiles(
	context.Context,
	outbound.Workspace,
) ([]outbound.WorkspaceStagedFile, error) {
	return append([]outbound.WorkspaceStagedFile(nil), m.staged...), m.err
}
