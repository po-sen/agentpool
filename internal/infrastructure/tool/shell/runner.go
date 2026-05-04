package shell

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

const (
	toolNameRunShell = "run_shell"

	defaultCommandTimeout = 10 * time.Second
	defaultMaxTimeout     = 30 * time.Second

	argumentCommand        = "command"
	argumentTimeoutSeconds = "timeout_seconds"
)

// Config controls sandbox shell tool limits.
type Config struct {
	DefaultTimeout time.Duration
	MaxTimeout     time.Duration
}

// Runner exposes sandbox-backed shell tools.
type Runner struct {
	commands       outbound.SandboxCommandRunner
	defaultTimeout time.Duration
	maxTimeout     time.Duration
}

var _ outbound.ToolRunner = (*Runner)(nil)

// NewRunner creates a sandbox shell tool runner.
func NewRunner(commands outbound.SandboxCommandRunner, cfg Config) (*Runner, error) {
	if commands == nil {
		return nil, errors.New("sandbox command runner is required")
	}

	defaultTimeout := cfg.DefaultTimeout
	if defaultTimeout <= 0 {
		defaultTimeout = defaultCommandTimeout
	}
	maxTimeout := cfg.MaxTimeout
	if maxTimeout <= 0 {
		maxTimeout = defaultMaxTimeout
	}
	if defaultTimeout > maxTimeout {
		defaultTimeout = maxTimeout
	}

	return &Runner{
		commands:       commands,
		defaultTimeout: defaultTimeout,
		maxTimeout:     maxTimeout,
	}, nil
}

// ListTools exposes run_shell only when command-capable sandbox and workspace context exists.
func (r *Runner) ListTools(_ context.Context, request outbound.ToolListRequest) ([]outbound.ToolDefinition, error) {
	if request.Context.Sandbox.ID == "" ||
		!request.Context.Sandbox.SupportsCommands ||
		request.Context.WorkspacePath == "" {
		return nil, nil
	}

	return []outbound.ToolDefinition{
		{
			Name:        toolNameRunShell,
			Description: "Runs a command inside the prepared sandbox workspace.",
		},
	}, nil
}

// RunTool executes a sandbox-backed shell command.
func (r *Runner) RunTool(ctx context.Context, call outbound.ToolCall) (outbound.ToolResult, error) {
	if call.Name != toolNameRunShell {
		return outbound.ToolResult{Content: fmt.Sprintf("unknown tool: %s", call.Name), IsError: true}, nil
	}
	if call.Context.Sandbox.ID == "" || !call.Context.Sandbox.SupportsCommands {
		return outbound.ToolResult{Content: "sandbox is not available", IsError: true}, nil
	}
	if call.Context.WorkspacePath == "" {
		return outbound.ToolResult{Content: "workspace path is not available", IsError: true}, nil
	}

	command := strings.TrimSpace(call.Arguments[argumentCommand])
	if command == "" {
		return outbound.ToolResult{Content: "missing command argument", IsError: true}, nil
	}

	timeout, err := r.parseTimeout(call.Arguments)
	if err != nil {
		return outbound.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := r.commands.RunCommand(commandCtx, outbound.SandboxCommandRequest{
		Sandbox:       call.Context.Sandbox,
		WorkspacePath: call.Context.WorkspacePath,
		Command:       command,
		Timeout:       timeout,
	})
	if err != nil {
		return outbound.ToolResult{}, err
	}

	return outbound.ToolResult{
		Content: formatCommandResult(result),
		IsError: result.TimedOut || result.ExitCode != 0,
	}, nil
}

func (r *Runner) parseTimeout(arguments map[string]string) (time.Duration, error) {
	raw := strings.TrimSpace(arguments[argumentTimeoutSeconds])
	if raw == "" {
		return r.defaultTimeout, nil
	}

	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return 0, errors.New("timeout_seconds must be a positive integer")
	}
	timeout := time.Duration(seconds) * time.Second
	if timeout > r.maxTimeout {
		return 0, fmt.Errorf("timeout_seconds exceeds maximum %d", int(r.maxTimeout/time.Second))
	}

	return timeout, nil
}

func formatCommandResult(result outbound.SandboxCommandResult) string {
	var output strings.Builder
	_, _ = fmt.Fprintf(&output, "exit_code: %d\n", result.ExitCode)
	if result.TimedOut {
		output.WriteString("timed_out: true\n")
	}
	output.WriteString("stdout:\n")
	writeBlock(&output, result.Stdout)
	output.WriteString("stderr:\n")
	writeBlock(&output, result.Stderr)

	return output.String()
}

func writeBlock(output *strings.Builder, content string) {
	output.WriteString(content)
	if !strings.HasSuffix(content, "\n") {
		output.WriteString("\n")
	}
}
