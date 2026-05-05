package registry

import (
	"context"
	"fmt"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

// Registry dispatches tool calls to context-available tool runners.
type Registry struct {
	runners []outbound.ToolRunner
}

var _ outbound.ToolRunner = (*Registry)(nil)

// New creates a tool registry.
func New(runners ...outbound.ToolRunner) (*Registry, error) {
	return &Registry{runners: runners}, nil
}

// ListTools combines tool definitions from every runner.
func (r *Registry) ListTools(ctx context.Context, request outbound.ToolListRequest) ([]outbound.ToolDefinition, error) {
	tools, _, err := r.toolsForContext(ctx, request)
	if err != nil {
		return nil, err
	}

	return tools, nil
}

// RunTool dispatches a tool call to the runner that owns the tool name.
func (r *Registry) RunTool(ctx context.Context, call outbound.ToolCall) (outbound.ToolResult, error) {
	_, owners, err := r.toolsForContext(ctx, outbound.ToolListRequest{
		RunID:   call.RunID,
		Context: call.Context,
	})
	if err != nil {
		return outbound.ToolResult{}, err
	}

	owner := owners[call.Name]
	if owner == nil {
		return outbound.ToolResult{Content: fmt.Sprintf("unknown tool: %s", call.Name), IsError: true}, nil
	}

	return owner.RunTool(ctx, call)
}

func (r *Registry) toolsForContext(
	ctx context.Context,
	request outbound.ToolListRequest,
) ([]outbound.ToolDefinition, map[string]outbound.ToolRunner, error) {
	var tools []outbound.ToolDefinition
	owners := make(map[string]outbound.ToolRunner)
	for _, runner := range r.runners {
		runnerTools, err := runner.ListTools(ctx, request)
		if err != nil {
			return nil, nil, err
		}
		for _, tool := range runnerTools {
			if _, exists := owners[tool.Name]; exists {
				return nil, nil, fmt.Errorf("duplicate tool name: %s", tool.Name)
			}
			owners[tool.Name] = runner
		}
		tools = append(tools, runnerTools...)
	}

	return tools, owners, nil
}
