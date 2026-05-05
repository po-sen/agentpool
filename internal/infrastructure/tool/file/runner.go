package file

import (
	"context"
	"fmt"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

const (
	toolNameListFiles = "list_files"
	toolNameReadFile  = "read_file"

	argumentPath = "path"

	defaultMaxFiles         = 200
	defaultMaxReadSizeBytes = 256 << 10
)

// Config controls read-only workspace file tool limits.
type Config struct {
	MaxFiles         int
	MaxReadSizeBytes int64
}

// Runner exposes read-only workspace file tools.
type Runner struct {
	maxFiles         int
	maxReadSizeBytes int64
}

var _ outbound.ToolRunner = (*Runner)(nil)

// NewRunner creates a read-only workspace file tool runner.
func NewRunner(cfg Config) *Runner {
	maxFiles := cfg.MaxFiles
	if maxFiles <= 0 {
		maxFiles = defaultMaxFiles
	}
	maxReadSizeBytes := cfg.MaxReadSizeBytes
	if maxReadSizeBytes <= 0 {
		maxReadSizeBytes = defaultMaxReadSizeBytes
	}

	return &Runner{
		maxFiles:         maxFiles,
		maxReadSizeBytes: maxReadSizeBytes,
	}
}

// ListTools exposes file tools only when uploaded files are available in the workspace.
func (r *Runner) ListTools(_ context.Context, request outbound.ToolListRequest) ([]outbound.ToolDefinition, error) {
	if request.Context.WorkspacePath == "" || !request.Context.WorkspaceHasFiles {
		return nil, nil
	}

	return []outbound.ToolDefinition{
		{
			Name:        toolNameListFiles,
			Description: "Lists uploaded workspace files as relative paths.",
		},
		{
			Name:        toolNameReadFile,
			Description: "Reads one UTF-8 text file from the uploaded workspace. Argument: path.",
		},
	}, nil
}

// RunTool executes a read-only workspace file tool call.
func (r *Runner) RunTool(ctx context.Context, call outbound.ToolCall) (outbound.ToolResult, error) {
	if call.Context.WorkspacePath == "" {
		return outbound.ToolResult{Content: "workspace path is not available", IsError: true}, nil
	}

	switch call.Name {
	case toolNameListFiles:
		return r.listFiles(ctx, call.Context.WorkspacePath), nil
	case toolNameReadFile:
		return r.readFile(call.Context.WorkspacePath, call.Arguments[argumentPath]), nil
	default:
		return outbound.ToolResult{Content: fmt.Sprintf("unknown tool: %s", call.Name), IsError: true}, nil
	}
}
