package composite

import (
	"context"
	"errors"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestRunnerImplementsToolRunner(t *testing.T) {
	runner, err := NewRunner(fakeToolRunner{name: "echo"})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	var _ outbound.ToolRunner = runner
}

func TestRunnerCombinesTools(t *testing.T) {
	runner, err := NewRunner(
		fakeToolRunner{name: "echo"},
		fakeToolRunner{name: "read_file"},
	)
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	tools, err := runner.ListTools(context.Background(), outbound.ToolListRequest{})
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("len(tools) = %d, want 2", len(tools))
	}
	if tools[0].Name != "echo" || tools[1].Name != "read_file" {
		t.Fatalf("tools = %#v, want echo and read_file", tools)
	}
}

func TestRunnerSupportsNoTools(t *testing.T) {
	runner, err := NewRunner()
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
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

func TestRunnerDispatchesToCorrectRunner(t *testing.T) {
	echoRunner := &recordingToolRunner{name: "echo", content: "echoed"}
	readRunner := &recordingToolRunner{name: "read_file", content: "read"}
	runner, err := NewRunner(echoRunner, readRunner)
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	result, err := runner.RunTool(context.Background(), outbound.ToolCall{Name: "read_file"})
	if err != nil {
		t.Fatalf("RunTool() error = %v", err)
	}
	if result.Content != "read" {
		t.Fatalf("Content = %q, want read", result.Content)
	}
	if echoRunner.calls != 0 {
		t.Fatalf("echo calls = %d, want 0", echoRunner.calls)
	}
	if readRunner.calls != 1 {
		t.Fatalf("read calls = %d, want 1", readRunner.calls)
	}
}

func TestRunnerListToolsRejectsDuplicateToolNames(t *testing.T) {
	runner, err := NewRunner(
		fakeToolRunner{name: "echo"},
		fakeToolRunner{name: "echo"},
	)
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	_, err = runner.ListTools(context.Background(), outbound.ToolListRequest{})
	if err == nil {
		t.Fatal("ListTools() error = nil, want duplicate error")
	}
}

func TestRunnerRunToolRejectsDuplicateToolNames(t *testing.T) {
	runner, err := NewRunner(
		fakeToolRunner{name: "echo"},
		fakeToolRunner{name: "echo"},
	)
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	_, err = runner.RunTool(context.Background(), outbound.ToolCall{Name: "echo"})
	if err == nil {
		t.Fatal("RunTool() error = nil, want duplicate error")
	}
}

func TestRunnerReturnsToolErrorForUnknownTool(t *testing.T) {
	runner, err := NewRunner(fakeToolRunner{name: "echo"})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
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

func TestRunnerPropagatesListToolsErrorsDuringConstruction(t *testing.T) {
	errListTools := errors.New("list tools failed")

	runner, err := NewRunner(errorToolRunner{err: errListTools})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}
	_, err = runner.ListTools(context.Background(), outbound.ToolListRequest{})
	if !errors.Is(err, errListTools) {
		t.Fatalf("ListTools() error = %v, want %v", err, errListTools)
	}
}

func TestRunnerUsesRequestContextForDynamicTools(t *testing.T) {
	runner, err := NewRunner(
		fakeToolRunner{name: "echo"},
		dynamicToolRunner{name: "read_file"},
	)
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	tools, err := runner.ListTools(context.Background(), outbound.ToolListRequest{})
	if err != nil {
		t.Fatalf("ListTools() without workspace error = %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("tools without workspace = %#v, want echo only", tools)
	}

	tools, err = runner.ListTools(context.Background(), outbound.ToolListRequest{
		Context: outbound.ToolContext{WorkspacePath: "/tmp/repo"},
	})
	if err != nil {
		t.Fatalf("ListTools() with workspace error = %v", err)
	}
	if len(tools) != 2 || tools[0].Name != "echo" || tools[1].Name != "read_file" {
		t.Fatalf("tools with workspace = %#v, want echo and read_file", tools)
	}
}

type fakeToolRunner struct {
	name string
}

func (r fakeToolRunner) ListTools(context.Context, outbound.ToolListRequest) ([]outbound.ToolDefinition, error) {
	return []outbound.ToolDefinition{{Name: r.name, Description: "test tool"}}, nil
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
	if request.Context.WorkspacePath == "" {
		return nil, nil
	}

	return []outbound.ToolDefinition{{Name: r.name, Description: "dynamic tool"}}, nil
}

func (r dynamicToolRunner) RunTool(context.Context, outbound.ToolCall) (outbound.ToolResult, error) {
	return outbound.ToolResult{Content: r.name}, nil
}
