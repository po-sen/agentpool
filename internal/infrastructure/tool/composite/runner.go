package composite

import (
	"context"
	"fmt"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

// Runner composes multiple tool runners behind one ToolRunner port.
type Runner struct {
	runners []outbound.ToolRunner
	owners  map[string]outbound.ToolRunner
}

var _ outbound.ToolRunner = (*Runner)(nil)

// NewRunner creates a composite tool runner.
func NewRunner(runners ...outbound.ToolRunner) (*Runner, error) {
	composite := &Runner{
		runners: runners,
		owners:  make(map[string]outbound.ToolRunner),
	}

	for _, runner := range runners {
		tools, err := runner.ListTools(context.Background(), outbound.ToolListRequest{})
		if err != nil {
			return nil, err
		}
		for _, tool := range tools {
			if _, exists := composite.owners[tool.Name]; exists {
				return nil, fmt.Errorf("duplicate tool name: %s", tool.Name)
			}
			composite.owners[tool.Name] = runner
		}
	}

	return composite, nil
}

// ListTools combines tool definitions from every runner.
func (r *Runner) ListTools(ctx context.Context, request outbound.ToolListRequest) ([]outbound.ToolDefinition, error) {
	var tools []outbound.ToolDefinition
	for _, runner := range r.runners {
		runnerTools, err := runner.ListTools(ctx, request)
		if err != nil {
			return nil, err
		}
		tools = append(tools, runnerTools...)
	}

	return tools, nil
}

// RunTool dispatches a tool call to the runner that owns the tool name.
func (r *Runner) RunTool(ctx context.Context, call outbound.ToolCall) (outbound.ToolResult, error) {
	runner, ok := r.owners[call.Name]
	if !ok {
		return outbound.ToolResult{
			Content: fmt.Sprintf("unknown tool: %s", call.Name),
			IsError: true,
		}, nil
	}

	return runner.RunTool(ctx, call)
}
