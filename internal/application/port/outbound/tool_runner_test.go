package outbound

import (
	"context"
	"testing"
)

func TestToolRunnerContract(t *testing.T) {
	var runner ToolRunner = contractToolRunner{}

	tools, err := runner.ListTools(context.Background(), ToolListRequest{})
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("len(tools) = %d, want 1", len(tools))
	}

	result, err := runner.RunTool(context.Background(), ToolCall{Name: "echo"})
	if err != nil {
		t.Fatalf("RunTool() error = %v", err)
	}
	if result.Content != "ok" {
		t.Fatalf("Content = %q, want ok", result.Content)
	}
}

type contractToolRunner struct{}

func (contractToolRunner) ListTools(context.Context, ToolListRequest) ([]ToolDefinition, error) {
	return []ToolDefinition{{Name: "echo", Description: "test tool"}}, nil
}

func (contractToolRunner) RunTool(context.Context, ToolCall) (ToolResult, error) {
	return ToolResult{Content: "ok"}, nil
}
