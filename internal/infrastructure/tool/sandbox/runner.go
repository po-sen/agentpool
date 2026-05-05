package sandbox

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

const (
	toolNameSandboxExec = "sandbox_exec"

	defaultCommandTimeout = 10 * time.Second
	defaultMaxTimeout     = 30 * time.Second

	defaultMaxOutputBytes = 64 << 10
	defaultOutputLimit    = 256 << 10

	argumentCommand        = "command"
	argumentTimeoutSeconds = "timeout_seconds"
	argumentMaxOutputBytes = "max_output_bytes"
)

// Config controls sandbox_exec limits.
type Config struct {
	DefaultTimeout        time.Duration
	MaxTimeout            time.Duration
	DefaultMaxOutputBytes int
	MaxOutputBytes        int
}

// Runner exposes sandbox-backed command execution.
type Runner struct {
	commands              outbound.SandboxCommandRunner
	defaultTimeout        time.Duration
	maxTimeout            time.Duration
	defaultMaxOutputBytes int
	maxOutputBytes        int
}

var _ outbound.ToolRunner = (*Runner)(nil)

// NewRunner creates a sandbox_exec tool runner.
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

	maxOutputBytes := cfg.MaxOutputBytes
	if maxOutputBytes <= 0 {
		maxOutputBytes = defaultOutputLimit
	}
	defaultOutputBytes := cfg.DefaultMaxOutputBytes
	if defaultOutputBytes <= 0 {
		defaultOutputBytes = defaultMaxOutputBytes
	}
	if defaultOutputBytes > maxOutputBytes {
		defaultOutputBytes = maxOutputBytes
	}

	return &Runner{
		commands:              commands,
		defaultTimeout:        defaultTimeout,
		maxTimeout:            maxTimeout,
		defaultMaxOutputBytes: defaultOutputBytes,
		maxOutputBytes:        maxOutputBytes,
	}, nil
}

// ListTools exposes sandbox_exec only when command-capable sandbox and workspace context exist.
func (r *Runner) ListTools(_ context.Context, request outbound.ToolListRequest) ([]outbound.ToolDefinition, error) {
	if !sandboxExecAvailable(request.Context) {
		return nil, nil
	}

	return []outbound.ToolDefinition{
		{
			Name:        toolNameSandboxExec,
			Description: "Runs a command inside the sandbox from /workspace/work.",
			Arguments: []outbound.ToolArgumentDefinition{
				{
					Name:        argumentCommand,
					Description: "Command to run inside the sandbox.",
					Required:    true,
					Example:     "wc -l /workspace/input/README.md",
				},
				{
					Name:        argumentTimeoutSeconds,
					Description: "Optional timeout in seconds. Must be a positive integer and no more than the configured maximum.",
					Required:    false,
					Example:     "10",
				},
				{
					Name:        argumentMaxOutputBytes,
					Description: "Optional maximum combined stdout and stderr bytes returned by the tool.",
					Required:    false,
					Example:     "65536",
				},
			},
		},
	}, nil
}

// RunTool executes sandbox_exec.
func (r *Runner) RunTool(ctx context.Context, call outbound.ToolCall) (outbound.ToolResult, error) {
	if call.Name != toolNameSandboxExec {
		return outbound.ToolResult{Content: fmt.Sprintf("unknown tool: %s", call.Name), IsError: true}, nil
	}
	if call.Context.Sandbox.ID == "" || !call.Context.Sandbox.SupportsCommands {
		return outbound.ToolResult{Content: "sandbox is not available", IsError: true}, nil
	}
	if !workspaceAvailable(call.Context.Workspace) {
		return outbound.ToolResult{Content: "workspace is not available", IsError: true}, nil
	}

	command := strings.TrimSpace(call.Arguments[argumentCommand])
	if command == "" {
		return outbound.ToolResult{Content: "missing command argument", IsError: true}, nil
	}

	timeout, err := r.parseTimeout(call.Arguments)
	if err != nil {
		return outbound.ToolResult{Content: err.Error(), IsError: true}, nil
	}
	maxOutputBytes, err := r.parseMaxOutputBytes(call.Arguments)
	if err != nil {
		return outbound.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := r.commands.RunCommand(commandCtx, outbound.SandboxCommandRequest{
		Sandbox:   call.Context.Sandbox,
		Workspace: call.Context.Workspace,
		Command:   command,
		Timeout:   timeout,
	})
	if err != nil {
		return outbound.ToolResult{}, err
	}

	return outbound.ToolResult{
		Content: formatCommandResult(result, maxOutputBytes),
		IsError: result.TimedOut || result.ExitCode != 0,
	}, nil
}

func sandboxExecAvailable(context outbound.ToolContext) bool {
	return context.Sandbox.ID != "" &&
		context.Sandbox.SupportsCommands &&
		workspaceAvailable(context.Workspace)
}

func workspaceAvailable(workspace outbound.Workspace) bool {
	return strings.TrimSpace(workspace.InputPath) != "" && strings.TrimSpace(workspace.WorkPath) != ""
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

func (r *Runner) parseMaxOutputBytes(arguments map[string]string) (int, error) {
	raw := strings.TrimSpace(arguments[argumentMaxOutputBytes])
	if raw == "" {
		return r.defaultMaxOutputBytes, nil
	}

	maxOutputBytes, err := strconv.Atoi(raw)
	if err != nil || maxOutputBytes <= 0 {
		return 0, errors.New("max_output_bytes must be a positive integer")
	}
	if maxOutputBytes > r.maxOutputBytes {
		return 0, fmt.Errorf("max_output_bytes exceeds maximum %d", r.maxOutputBytes)
	}

	return maxOutputBytes, nil
}

func formatCommandResult(result outbound.SandboxCommandResult, maxOutputBytes int) string {
	stdout, stderr, truncated := truncateStreams(result.Stdout, result.Stderr, maxOutputBytes)

	var output strings.Builder
	_, _ = fmt.Fprintf(&output, "exit_code: %d\n", result.ExitCode)
	_, _ = fmt.Fprintf(&output, "timed_out: %t\n", result.TimedOut)
	output.WriteString("stdout:\n")
	writeBlock(&output, stdout)
	output.WriteString("stderr:\n")
	writeBlock(&output, stderr)
	if truncated {
		output.WriteString("truncated: true\n")
	}

	return output.String()
}

func truncateStreams(stdout string, stderr string, maxBytes int) (string, string, bool) {
	if stdout == "" {
		stderr, truncated := truncateString(stderr, maxBytes)

		return stdout, stderr, truncated
	}
	if stderr == "" {
		stdout, truncated := truncateString(stdout, maxBytes)

		return stdout, stderr, truncated
	}

	stdoutBudget := maxBytes / 2
	stderrBudget := maxBytes - stdoutBudget

	stdout, stdoutTruncated := truncateString(stdout, stdoutBudget)
	stderr, stderrTruncated := truncateString(stderr, stderrBudget)

	return stdout, stderr, stdoutTruncated || stderrTruncated
}

func truncateString(content string, maxBytes int) (string, bool) {
	if maxBytes <= 0 {
		return "", content != ""
	}
	if len(content) <= maxBytes {
		return content, false
	}

	end := 0
	for index := range content {
		if index > maxBytes {
			break
		}
		end = index
	}
	if end == 0 {
		_, size := utf8.DecodeRuneInString(content)
		if size > maxBytes {
			return "", true
		}
		end = size
	}

	return content[:end], true
}

func writeBlock(output *strings.Builder, content string) {
	output.WriteString(content)
	if !strings.HasSuffix(content, "\n") {
		output.WriteString("\n")
	}
}
