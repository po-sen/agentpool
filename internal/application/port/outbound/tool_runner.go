package outbound

import (
	"context"

	"github.com/po-sen/agentpool/internal/domain/run"
)

// ToolDefinition describes an application-visible tool.
type ToolDefinition struct {
	Name        string
	Description string
}

// ToolListRequest contains context needed to list available tools.
type ToolListRequest struct {
	RunID   run.RunID
	Sandbox Sandbox
}

// ToolCall describes a provider-neutral tool invocation.
type ToolCall struct {
	RunID     run.RunID
	Sandbox   Sandbox
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
