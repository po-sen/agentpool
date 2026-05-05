package shell

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestRunnerImplementsToolRunner(t *testing.T) {
	runner := newTestRunner(t, &recordingCommandRunner{})

	var _ outbound.ToolRunner = runner
}

func TestNewRunnerRejectsMissingCommandRunner(t *testing.T) {
	_, err := NewRunner(nil, Config{})
	if err == nil {
		t.Fatal("NewRunner() error = nil, want error")
	}
}

func TestListToolsRequiresSandboxAndWorkspace(t *testing.T) {
	runner := newTestRunner(t, &recordingCommandRunner{})

	tools, err := runner.ListTools(context.Background(), outbound.ToolListRequest{})
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools) != 0 {
		t.Fatalf("len(tools) = %d, want 0", len(tools))
	}

	tools, err = runner.ListTools(context.Background(), outbound.ToolListRequest{
		Context: outbound.ToolContext{Sandbox: outbound.Sandbox{ID: "sandbox_test"}},
	})
	if err != nil {
		t.Fatalf("ListTools() sandbox only error = %v", err)
	}
	if len(tools) != 0 {
		t.Fatalf("len(tools) with sandbox only = %d, want 0", len(tools))
	}

	tools, err = runner.ListTools(context.Background(), outbound.ToolListRequest{
		Context: outbound.ToolContext{
			WorkspacePath: "/tmp/workspace",
			Sandbox:       outbound.Sandbox{ID: "sandbox_test"},
		},
	})
	if err != nil {
		t.Fatalf("ListTools() without command support error = %v", err)
	}
	if len(tools) != 0 {
		t.Fatalf("len(tools) without command support = %d, want 0", len(tools))
	}

	tools, err = runner.ListTools(context.Background(), outbound.ToolListRequest{
		Context: outbound.ToolContext{
			WorkspacePath: "/tmp/workspace",
			Sandbox:       commandCapableSandbox(),
		},
	})
	if err != nil {
		t.Fatalf("ListTools() full context error = %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "run_shell" {
		t.Fatalf("tools = %#v, want run_shell", tools)
	}
	if len(tools[0].Arguments) != 2 {
		t.Fatalf("run_shell arguments = %#v, want command and timeout_seconds", tools[0].Arguments)
	}
	commandArgument := tools[0].Arguments[0]
	if commandArgument.Name != argumentCommand ||
		commandArgument.Description != "Shell command to run inside the prepared sandbox workspace." ||
		!commandArgument.Required ||
		commandArgument.Example != "pwd && ls -la" {
		t.Fatalf("run_shell command argument = %#v, want required command metadata", commandArgument)
	}
	timeoutArgument := tools[0].Arguments[1]
	if timeoutArgument.Name != argumentTimeoutSeconds ||
		timeoutArgument.Description != "Optional timeout in seconds. Must be a positive integer and no more than the configured maximum." ||
		timeoutArgument.Required ||
		timeoutArgument.Example != "10" {
		t.Fatalf("run_shell timeout argument = %#v, want optional timeout_seconds metadata", timeoutArgument)
	}
}

func TestListToolsAllowsEmptyWorkspaceWithCommandSandbox(t *testing.T) {
	runner := newTestRunner(t, &recordingCommandRunner{})

	tools, err := runner.ListTools(context.Background(), outbound.ToolListRequest{
		Context: outbound.ToolContext{
			WorkspacePath:     "/tmp/workspace",
			WorkspaceHasFiles: false,
			Sandbox:           commandCapableSandbox(),
		},
	})
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "run_shell" {
		t.Fatalf("tools = %#v, want run_shell", tools)
	}
}

func TestRunToolExecutesSandboxCommand(t *testing.T) {
	commands := &recordingCommandRunner{
		result: outbound.SandboxCommandResult{
			Stdout:   "hello\n",
			ExitCode: 0,
		},
	}
	runner := newTestRunner(t, commands)

	result, err := runner.RunTool(context.Background(), outbound.ToolCall{
		Name: toolNameRunShell,
		Context: outbound.ToolContext{
			WorkspacePath: "/tmp/workspace",
			Sandbox:       commandCapableSandbox(),
		},
		Arguments: map[string]string{
			"command":         " echo hello ",
			"timeout_seconds": "3",
		},
	})
	if err != nil {
		t.Fatalf("RunTool() error = %v", err)
	}
	if result.IsError {
		t.Fatal("IsError = true, want false")
	}
	if commands.calls != 1 {
		t.Fatalf("command calls = %d, want 1", commands.calls)
	}
	if commands.request.Command != "echo hello" {
		t.Fatalf("command = %q, want echo hello", commands.request.Command)
	}
	if commands.request.WorkspacePath != "/tmp/workspace" {
		t.Fatalf("workspace path = %q, want /tmp/workspace", commands.request.WorkspacePath)
	}
	if !commands.request.Sandbox.SupportsCommands {
		t.Fatal("sandbox command support = false, want true")
	}
	if commands.request.Timeout != 3*time.Second {
		t.Fatalf("timeout = %s, want 3s", commands.request.Timeout)
	}
	if !strings.Contains(result.Content, "exit_code: 0") || !strings.Contains(result.Content, "hello") {
		t.Fatalf("content = %q, want exit code and stdout", result.Content)
	}
}

func TestRunToolRejectsMissingContextAndArguments(t *testing.T) {
	runner := newTestRunner(t, &recordingCommandRunner{})

	tests := []outbound.ToolCall{
		{Name: toolNameRunShell},
		{
			Name: toolNameRunShell,
			Context: outbound.ToolContext{
				Sandbox: outbound.Sandbox{ID: "sandbox_test"},
			},
		},
		{
			Name: toolNameRunShell,
			Context: outbound.ToolContext{
				WorkspacePath: "/tmp/workspace",
				Sandbox:       outbound.Sandbox{ID: "sandbox_test"},
			},
		},
		{
			Name: toolNameRunShell,
			Context: outbound.ToolContext{
				WorkspacePath: "/tmp/workspace",
				Sandbox:       commandCapableSandbox(),
			},
		},
	}
	for _, test := range tests {
		result, err := runner.RunTool(context.Background(), test)
		if err != nil {
			t.Fatalf("RunTool() error = %v", err)
		}
		if !result.IsError {
			t.Fatalf("RunTool(%#v) IsError = false, want true", test)
		}
	}
}

func TestRunToolReportsSandboxUnavailableWithoutCommandSupport(t *testing.T) {
	commands := &recordingCommandRunner{}
	runner := newTestRunner(t, commands)

	result, err := runner.RunTool(context.Background(), outbound.ToolCall{
		Name: toolNameRunShell,
		Context: outbound.ToolContext{
			WorkspacePath: "/tmp/workspace",
			Sandbox:       outbound.Sandbox{ID: "sandbox_test"},
		},
		Arguments: map[string]string{"command": "echo hello"},
	})
	if err != nil {
		t.Fatalf("RunTool() error = %v", err)
	}
	if !result.IsError {
		t.Fatal("IsError = false, want true")
	}
	if result.Content != "sandbox is not available" {
		t.Fatalf("Content = %q, want sandbox is not available", result.Content)
	}
	if commands.calls != 0 {
		t.Fatalf("command calls = %d, want 0", commands.calls)
	}
}

func TestRunToolRejectsInvalidTimeout(t *testing.T) {
	runner := newTestRunner(t, &recordingCommandRunner{})

	result, err := runner.RunTool(context.Background(), outbound.ToolCall{
		Name: toolNameRunShell,
		Context: outbound.ToolContext{
			WorkspacePath: "/tmp/workspace",
			Sandbox:       commandCapableSandbox(),
		},
		Arguments: map[string]string{
			"command":         "echo hello",
			"timeout_seconds": "31",
		},
	})
	if err != nil {
		t.Fatalf("RunTool() error = %v", err)
	}
	if !result.IsError {
		t.Fatal("IsError = false, want true")
	}
	if !strings.Contains(result.Content, "timeout_seconds exceeds maximum") {
		t.Fatalf("Content = %q, want max timeout error", result.Content)
	}
}

func TestRunToolMarksNonZeroExitAsToolError(t *testing.T) {
	runner := newTestRunner(t, &recordingCommandRunner{
		result: outbound.SandboxCommandResult{
			Stderr:   "failed",
			ExitCode: 2,
		},
	})

	result, err := runner.RunTool(context.Background(), outbound.ToolCall{
		Name: toolNameRunShell,
		Context: outbound.ToolContext{
			WorkspacePath: "/tmp/workspace",
			Sandbox:       commandCapableSandbox(),
		},
		Arguments: map[string]string{"command": "false"},
	})
	if err != nil {
		t.Fatalf("RunTool() error = %v", err)
	}
	if !result.IsError {
		t.Fatal("IsError = false, want true")
	}
	if !strings.Contains(result.Content, "exit_code: 2") || !strings.Contains(result.Content, "failed") {
		t.Fatalf("Content = %q, want exit code and stderr", result.Content)
	}
}

func TestRunToolPropagatesCommandRunnerError(t *testing.T) {
	errCommand := errors.New("command runner failed")
	runner := newTestRunner(t, &recordingCommandRunner{err: errCommand})

	_, err := runner.RunTool(context.Background(), outbound.ToolCall{
		Name: toolNameRunShell,
		Context: outbound.ToolContext{
			WorkspacePath: "/tmp/workspace",
			Sandbox:       commandCapableSandbox(),
		},
		Arguments: map[string]string{"command": "echo hello"},
	})
	if !errors.Is(err, errCommand) {
		t.Fatalf("RunTool() error = %v, want %v", err, errCommand)
	}
}

func TestRunToolReturnsToolErrorForUnknownTool(t *testing.T) {
	runner := newTestRunner(t, &recordingCommandRunner{})

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

func newTestRunner(t *testing.T, commands outbound.SandboxCommandRunner) *Runner {
	t.Helper()

	runner, err := NewRunner(commands, Config{})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	return runner
}

func commandCapableSandbox() outbound.Sandbox {
	return outbound.Sandbox{ID: "sandbox_test", SupportsCommands: true}
}

type recordingCommandRunner struct {
	calls   int
	request outbound.SandboxCommandRequest
	result  outbound.SandboxCommandResult
	err     error
}

func (r *recordingCommandRunner) RunCommand(
	_ context.Context,
	request outbound.SandboxCommandRequest,
) (outbound.SandboxCommandResult, error) {
	r.calls++
	r.request = request

	return r.result, r.err
}
