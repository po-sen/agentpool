package builtin

import (
	"context"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestRunnerImplementsToolRunner(_ *testing.T) {
	var _ outbound.ToolRunner = NewRunner()
}

func TestRunnerListsEchoTool(t *testing.T) {
	runner := NewRunner()

	tools, err := runner.ListTools(context.Background(), outbound.ToolListRequest{})
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("len(tools) = %d, want 1", len(tools))
	}
	if tools[0].Name != "echo" {
		t.Fatalf("tool name = %q, want echo", tools[0].Name)
	}
	if tools[0].Description == "" {
		t.Fatal("tool description is empty")
	}
}

func TestRunnerEchoReturnsText(t *testing.T) {
	runner := NewRunner()

	result, err := runner.RunTool(context.Background(), outbound.ToolCall{
		Name:      "echo",
		Arguments: map[string]string{"text": "hello"},
	})
	if err != nil {
		t.Fatalf("RunTool() error = %v", err)
	}
	if result.IsError {
		t.Fatal("IsError = true, want false")
	}
	if result.Content != "hello" {
		t.Fatalf("Content = %q, want hello", result.Content)
	}
}

func TestRunnerEchoMissingTextReturnsToolError(t *testing.T) {
	runner := NewRunner()

	result, err := runner.RunTool(context.Background(), outbound.ToolCall{
		Name:      "echo",
		Arguments: map[string]string{},
	})
	if err != nil {
		t.Fatalf("RunTool() error = %v", err)
	}
	if !result.IsError {
		t.Fatal("IsError = false, want true")
	}
	if result.Content != "missing text argument" {
		t.Fatalf("Content = %q, want missing text argument", result.Content)
	}
}

func TestRunnerUnknownToolReturnsToolError(t *testing.T) {
	runner := NewRunner()

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
