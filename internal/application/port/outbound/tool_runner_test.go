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
	if len(tools[0].Arguments) != 1 || tools[0].Arguments[0].Name != "text" {
		t.Fatalf("tool arguments = %#v, want text argument", tools[0].Arguments)
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
	return []ToolDefinition{
		{
			Name:        "echo",
			Description: "test tool",
			Arguments: []ToolArgumentDefinition{
				{Name: "text", Description: "Text to echo.", Required: true, Example: "hello"},
			},
		},
	}, nil
}

func (contractToolRunner) RunTool(context.Context, ToolCall) (ToolResult, error) {
	return ToolResult{Content: "ok"}, nil
}
