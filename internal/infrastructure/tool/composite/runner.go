package composite

import (
	"context"
	"fmt"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

// Runner composes multiple tool runners behind one ToolRunner port.
type Runner struct {
	runners []outbound.ToolRunner
}

var _ outbound.ToolRunner = (*Runner)(nil)

// NewRunner creates a composite tool runner.
func NewRunner(runners ...outbound.ToolRunner) (*Runner, error) {
	return &Runner{runners: runners}, nil
}

// ListTools combines tool definitions from every runner.
func (r *Runner) ListTools(ctx context.Context, request outbound.ToolListRequest) ([]outbound.ToolDefinition, error) {
	var tools []outbound.ToolDefinition
	owners := make(map[string]struct{})
	for _, runner := range r.runners {
		runnerTools, err := runner.ListTools(ctx, request)
		if err != nil {
			return nil, err
		}
		for _, tool := range runnerTools {
			if _, exists := owners[tool.Name]; exists {
				return nil, fmt.Errorf("duplicate tool name: %s", tool.Name)
			}
			owners[tool.Name] = struct{}{}
		}
		tools = append(tools, runnerTools...)
	}

	return tools, nil
}

// RunTool dispatches a tool call to the runner that owns the tool name.
func (r *Runner) RunTool(ctx context.Context, call outbound.ToolCall) (outbound.ToolResult, error) {
	var owner outbound.ToolRunner
	for _, runner := range r.runners {
		tools, err := runner.ListTools(ctx, outbound.ToolListRequest{
			RunID:   call.RunID,
			Context: call.Context,
		})
		if err != nil {
			return outbound.ToolResult{}, err
		}
		for _, tool := range tools {
			if tool.Name != call.Name {
				continue
			}
			if owner != nil {
				return outbound.ToolResult{}, fmt.Errorf("duplicate tool name: %s", call.Name)
			}
			owner = runner
		}
	}
	if owner == nil {
		return outbound.ToolResult{Content: fmt.Sprintf("unknown tool: %s", call.Name), IsError: true}, nil
	}

	return owner.RunTool(ctx, call)
}
