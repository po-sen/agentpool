package registry

import (
	"context"
	"errors"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestRegistryImplementsToolRunner(t *testing.T) {
	runner, err := New(fakeToolRunner{name: "echo"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var _ outbound.ToolRunner = runner
}

func TestRegistryCombinesTools(t *testing.T) {
	runner, err := New(
		fakeToolRunner{name: "echo"},
		fakeToolRunner{name: "workspace"},
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	tools, err := runner.ListTools(context.Background(), outbound.ToolListRequest{})
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("len(tools) = %d, want 2", len(tools))
	}
	if tools[0].Name != "echo" || tools[1].Name != "workspace" {
		t.Fatalf("tools = %#v, want echo and workspace", tools)
	}
}

func TestRegistryPreservesToolArgumentMetadata(t *testing.T) {
	arguments := []outbound.ToolArgumentDefinition{
		{
			Name:        "operation",
			Description: "Workspace metadata operation.",
			Required:    true,
			Example:     "list",
		},
	}
	runner, err := New(fakeToolRunner{name: "workspace", arguments: arguments})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	tools, err := runner.ListTools(context.Background(), outbound.ToolListRequest{})
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("len(tools) = %d, want 1", len(tools))
	}
	if len(tools[0].Arguments) != 1 || tools[0].Arguments[0] != arguments[0] {
		t.Fatalf("tool arguments = %#v, want %#v", tools[0].Arguments, arguments)
	}
}

func TestRegistrySupportsNoTools(t *testing.T) {
	runner, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	tools, err := runner.ListTools(context.Background(), outbound.ToolListRequest{})
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools) != 0 {
		t.Fatalf("len(tools) = %d, want 0", len(tools))
	}

	result, err := runner.RunTool(context.Background(), outbound.ToolCall{Name: "missing"})
	if err != nil {
		t.Fatalf("RunTool() error = %v", err)
	}
	if !result.IsError {
		t.Fatal("IsError = false, want true")
	}
	if result.Content != "unknown tool: missing" {
		t.Fatalf("Content = %q, want unknown tool: missing", result.Content)
	}
}

func TestRegistryDispatchesToCorrectRunner(t *testing.T) {
	echoRunner := &recordingToolRunner{name: "echo", content: "echoed"}
	workspaceRunner := &recordingToolRunner{name: "workspace", content: "files"}
	runner, err := New(echoRunner, workspaceRunner)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := runner.RunTool(context.Background(), outbound.ToolCall{Name: "workspace"})
	if err != nil {
		t.Fatalf("RunTool() error = %v", err)
	}
	if result.Content != "files" {
		t.Fatalf("Content = %q, want files", result.Content)
	}
	if echoRunner.calls != 0 {
		t.Fatalf("echo calls = %d, want 0", echoRunner.calls)
	}
	if workspaceRunner.calls != 1 {
		t.Fatalf("workspace calls = %d, want 1", workspaceRunner.calls)
	}
}

func TestRegistryDispatchesSandboxExec(t *testing.T) {
	workspaceRunner := &recordingToolRunner{name: "workspace", content: "files"}
	sandboxRunner := &recordingToolRunner{name: "sandbox_exec", content: "exit_code: 0"}
	runner, err := New(workspaceRunner, sandboxRunner)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := runner.RunTool(context.Background(), outbound.ToolCall{Name: "sandbox_exec"})
	if err != nil {
		t.Fatalf("RunTool() error = %v", err)
	}
	if result.Content != "exit_code: 0" {
		t.Fatalf("Content = %q, want command result", result.Content)
	}
	if workspaceRunner.calls != 0 {
		t.Fatalf("workspace calls = %d, want 0", workspaceRunner.calls)
	}
	if sandboxRunner.calls != 1 {
		t.Fatalf("sandbox calls = %d, want 1", sandboxRunner.calls)
	}
}

func TestRegistryListToolsRejectsDuplicateToolNames(t *testing.T) {
	runner, err := New(
		fakeToolRunner{name: "echo"},
		fakeToolRunner{name: "echo"},
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = runner.ListTools(context.Background(), outbound.ToolListRequest{})
	if err == nil {
		t.Fatal("ListTools() error = nil, want duplicate error")
	}
}

func TestRegistryRunToolRejectsDuplicateToolNames(t *testing.T) {
	runner, err := New(
		fakeToolRunner{name: "echo"},
		fakeToolRunner{name: "echo"},
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = runner.RunTool(context.Background(), outbound.ToolCall{Name: "echo"})
	if err == nil {
		t.Fatal("RunTool() error = nil, want duplicate error")
	}
}

func TestRegistryReturnsToolErrorForUnknownTool(t *testing.T) {
	runner, err := New(fakeToolRunner{name: "echo"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := runner.RunTool(context.Background(), outbound.ToolCall{Name: "missing"})
	if err != nil {
		t.Fatalf("RunTool() error = %v", err)
	}
	if !result.IsError {
		t.Fatal("IsError = false, want true")
	}
	if result.Content != "unknown tool: missing" {
		t.Fatalf("Content = %q, want unknown tool: missing", result.Content)
	}
}

func TestRegistryPropagatesListToolsErrors(t *testing.T) {
	errListTools := errors.New("list tools failed")

	runner, err := New(errorToolRunner{err: errListTools})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = runner.ListTools(context.Background(), outbound.ToolListRequest{})
	if !errors.Is(err, errListTools) {
		t.Fatalf("ListTools() error = %v, want %v", err, errListTools)
	}
}

func TestRegistryUsesRequestContextForDynamicTools(t *testing.T) {
	runner, err := New(
		fakeToolRunner{name: "echo"},
		dynamicToolRunner{name: "workspace"},
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	tools, err := runner.ListTools(context.Background(), outbound.ToolListRequest{})
	if err != nil {
		t.Fatalf("ListTools() without workspace error = %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("tools without workspace = %#v, want echo only", tools)
	}

	tools, err = runner.ListTools(context.Background(), outbound.ToolListRequest{
		Context: outbound.ToolContext{
			Workspace: testWorkspace(),
		},
	})
	if err != nil {
		t.Fatalf("ListTools() with workspace error = %v", err)
	}
	if len(tools) != 2 || tools[0].Name != "echo" || tools[1].Name != "workspace" {
		t.Fatalf("tools with workspace = %#v, want echo and workspace", tools)
	}
}

func TestRegistryUsesCallContextForDynamicToolDispatch(t *testing.T) {
	runner, err := New(dynamicToolRunner{name: "workspace"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := runner.RunTool(context.Background(), outbound.ToolCall{Name: "workspace"})
	if err != nil {
		t.Fatalf("RunTool() without workspace error = %v", err)
	}
	if !result.IsError || result.Content != "unknown tool: workspace" {
		t.Fatalf("RunTool() without workspace result = %#v, want unknown tool error", result)
	}

	result, err = runner.RunTool(context.Background(), outbound.ToolCall{
		Name: "workspace",
		Context: outbound.ToolContext{
			Workspace: testWorkspace(),
		},
	})
	if err != nil {
		t.Fatalf("RunTool() with workspace error = %v", err)
	}
	if result.IsError || result.Content != "workspace" {
		t.Fatalf("RunTool() with workspace result = %#v, want workspace content", result)
	}
}

type fakeToolRunner struct {
	name      string
	arguments []outbound.ToolArgumentDefinition
}

func (r fakeToolRunner) ListTools(context.Context, outbound.ToolListRequest) ([]outbound.ToolDefinition, error) {
	return []outbound.ToolDefinition{{Name: r.name, Description: "test tool", Arguments: r.arguments}}, nil
}

func (r fakeToolRunner) RunTool(context.Context, outbound.ToolCall) (outbound.ToolResult, error) {
	return outbound.ToolResult{Content: r.name}, nil
}

type recordingToolRunner struct {
	name    string
	content string
	calls   int
}

func (r *recordingToolRunner) ListTools(context.Context, outbound.ToolListRequest) ([]outbound.ToolDefinition, error) {
	return []outbound.ToolDefinition{{Name: r.name, Description: "test tool"}}, nil
}

func (r *recordingToolRunner) RunTool(context.Context, outbound.ToolCall) (outbound.ToolResult, error) {
	r.calls++

	return outbound.ToolResult{Content: r.content}, nil
}

type errorToolRunner struct {
	err error
}

func (r errorToolRunner) ListTools(context.Context, outbound.ToolListRequest) ([]outbound.ToolDefinition, error) {
	return nil, r.err
}

func (r errorToolRunner) RunTool(context.Context, outbound.ToolCall) (outbound.ToolResult, error) {
	return outbound.ToolResult{}, nil
}

type dynamicToolRunner struct {
	name string
}

func (r dynamicToolRunner) ListTools(_ context.Context, request outbound.ToolListRequest) ([]outbound.ToolDefinition, error) {
	if request.Context.Workspace.RootPath == "" {
		return nil, nil
	}

	return []outbound.ToolDefinition{{Name: r.name, Description: "dynamic tool"}}, nil
}

func (r dynamicToolRunner) RunTool(context.Context, outbound.ToolCall) (outbound.ToolResult, error) {
	return outbound.ToolResult{Content: r.name}, nil
}

func testWorkspace() outbound.Workspace {
	return outbound.Workspace{
		RootPath: "/tmp/repo",
	}
}
