package outbound_test

import (
	"context"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestToolRunnerContract(t *testing.T) {
	var runner outbound.ToolRunner = contractToolRunner{}

	tools, err := runner.ListTools(context.Background(), outbound.ToolListRequest{})
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("len(tools) = %d, want 1", len(tools))
	}

	result, err := runner.RunTool(context.Background(), outbound.ToolCall{Name: "echo"})
	if err != nil {
		t.Fatalf("RunTool() error = %v", err)
	}
	if result.Content != "ok" {
		t.Fatalf("Content = %q, want ok", result.Content)
	}
}

type contractToolRunner struct{}

func (contractToolRunner) ListTools(context.Context, outbound.ToolListRequest) ([]outbound.ToolDefinition, error) {
	return []outbound.ToolDefinition{{Name: "echo", Description: "test tool"}}, nil
}

func (contractToolRunner) RunTool(context.Context, outbound.ToolCall) (outbound.ToolResult, error) {
	return outbound.ToolResult{Content: "ok"}, nil
}
