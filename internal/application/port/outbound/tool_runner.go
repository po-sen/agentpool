package outbound

import (
	"context"

	"github.com/po-sen/agentpool/internal/domain/run"
)

// ToolDefinition describes a model-facing callable tool shape.
type ToolDefinition struct {
	Name        string
	Description string
	Arguments   []ToolArgumentDefinition
}

// ToolArgumentDefinition describes a model-facing string tool argument.
type ToolArgumentDefinition struct {
	Name        string
	Description string
	Required    bool
	Example     string
}

// ToolContext carries application execution context visible to tools.
type ToolContext struct {
	WorkspacePath     string
	WorkspaceHasFiles bool
	Sandbox           Sandbox
}

// ToolListRequest contains context needed to list available tools.
type ToolListRequest struct {
	RunID   run.RunID
	Context ToolContext
}

// ToolCall describes a provider-neutral tool invocation.
type ToolCall struct {
	RunID     run.RunID
	Context   ToolContext
	Name      string
	Arguments map[string]string
}

// ToolResult contains provider-neutral tool output.
type ToolResult struct {
	Content string
	IsError bool
}

// ToolRunner lists and executes application tools.
type ToolRunner interface {
	ListTools(context.Context, ToolListRequest) ([]ToolDefinition, error)
	RunTool(context.Context, ToolCall) (ToolResult, error)
}
