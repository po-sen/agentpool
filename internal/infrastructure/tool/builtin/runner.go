package builtin

import (
	"context"
	"fmt"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

const echoToolName = "echo"

// Runner executes safe builtin tools.
type Runner struct{}

var _ outbound.ToolRunner = (*Runner)(nil)

// NewRunner creates a builtin tool runner.
func NewRunner() *Runner {
	return &Runner{}
}

// ListTools returns the safe builtin tool definitions.
func (r *Runner) ListTools(context.Context, outbound.ToolListRequest) ([]outbound.ToolDefinition, error) {
	return []outbound.ToolDefinition{
		{
			Name:        echoToolName,
			Description: "Returns the provided text. Useful for verifying tool execution plumbing.",
		},
	}, nil
}

// RunTool executes a safe builtin tool.
func (r *Runner) RunTool(_ context.Context, call outbound.ToolCall) (outbound.ToolResult, error) {
	switch call.Name {
	case echoToolName:
		text := call.Arguments["text"]
		if text == "" {
			return outbound.ToolResult{Content: "missing text argument", IsError: true}, nil
		}

		return outbound.ToolResult{Content: text}, nil
	default:
		return outbound.ToolResult{Content: fmt.Sprintf("unknown tool: %s", call.Name), IsError: true}, nil
	}
}
