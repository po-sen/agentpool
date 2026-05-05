package sandbox

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestRunnerImplementsToolRunner(t *testing.T) {
	runner, err := NewRunner(&recordingCommandRunner{}, Config{})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	var _ outbound.ToolRunner = runner
}

func TestRunnerAdvertisesOnlySandboxExec(t *testing.T) {
	runner := newTestRunner(t)

	tools, err := runner.ListTools(context.Background(), outbound.ToolListRequest{
		Context: availableContext(),
	})
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools) != 1 || tools[0].Name != toolNameSandboxExec {
		t.Fatalf("tools = %#v, want sandbox_exec", tools)
	}
	if len(tools[0].Arguments) != 3 {
		t.Fatalf("arguments = %#v, want command, timeout_seconds, max_output_bytes", tools[0].Arguments)
	}
}

func TestRunnerDoesNotAdvertiseWithoutSandbox(t *testing.T) {
	runner := newTestRunner(t)

	tools, err := runner.ListTools(context.Background(), outbound.ToolListRequest{
		Context: outbound.ToolContext{Workspace: testWorkspace()},
	})
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools) != 0 {
		t.Fatalf("tools = %#v, want none", tools)
	}
}

func TestRunnerDoesNotAdvertiseWithoutCommandSupport(t *testing.T) {
	runner := newTestRunner(t)

	tools, err := runner.ListTools(context.Background(), outbound.ToolListRequest{
		Context: outbound.ToolContext{
			Workspace: testWorkspace(),
			Sandbox:   outbound.Sandbox{ID: "sandbox"},
		},
	})
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools) != 0 {
		t.Fatalf("tools = %#v, want none", tools)
	}
}

func TestRunnerDoesNotAdvertiseWithoutWorkspace(t *testing.T) {
	runner := newTestRunner(t)

	tools, err := runner.ListTools(context.Background(), outbound.ToolListRequest{
		Context: outbound.ToolContext{
			Sandbox: outbound.Sandbox{ID: "sandbox", SupportsCommands: true},
			Workspace: outbound.Workspace{
				InputPath: "/tmp/workspace/input",
			},
		},
	})
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools) != 0 {
		t.Fatalf("tools = %#v, want none", tools)
	}
}

func TestRunnerRequiresCommand(t *testing.T) {
	result := runSandboxExec(t, newTestRunner(t), availableContext(), nil)
	if !result.IsError || !strings.Contains(result.Content, "missing command argument") {
		t.Fatalf("result = %#v, want missing command error", result)
	}
}

func TestRunnerParsesTimeout(t *testing.T) {
	commands := &recordingCommandRunner{}
	runner, err := NewRunner(commands, Config{})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	result := runSandboxExec(t, runner, availableContext(), map[string]string{
		argumentCommand:        "pwd",
		argumentTimeoutSeconds: "3",
	})
	if result.IsError {
		t.Fatalf("result = %#v, want success", result)
	}
	if commands.request.Timeout != 3*time.Second {
		t.Fatalf("Timeout = %v, want 3s", commands.request.Timeout)
	}
}

func TestRunnerEnforcesMaxTimeout(t *testing.T) {
	result := runSandboxExec(t, newTestRunner(t), availableContext(), map[string]string{
		argumentCommand:        "pwd",
		argumentTimeoutSeconds: "31",
	})
	if !result.IsError || !strings.Contains(result.Content, "timeout_seconds exceeds maximum 30") {
		t.Fatalf("result = %#v, want max timeout error", result)
	}
}

func TestRunnerParsesMaxOutputBytes(t *testing.T) {
	commands := &recordingCommandRunner{
		result: outbound.SandboxCommandResult{Stdout: "hello\n"},
	}
	runner, err := NewRunner(commands, Config{})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	result := runSandboxExec(t, runner, availableContext(), map[string]string{
		argumentCommand:        "pwd",
		argumentMaxOutputBytes: "4",
	})
	if !strings.Contains(result.Content, "hell") || !strings.Contains(result.Content, "truncated: true") {
		t.Fatalf("result = %q, want truncated output", result.Content)
	}
}

func TestRunnerEnforcesMaxOutputLimit(t *testing.T) {
	result := runSandboxExec(t, newTestRunner(t), availableContext(), map[string]string{
		argumentCommand:        "pwd",
		argumentMaxOutputBytes: "262145",
	})
	if !result.IsError || !strings.Contains(result.Content, "max_output_bytes exceeds maximum 262144") {
		t.Fatalf("result = %#v, want max output error", result)
	}
}

func TestRunnerFormatsCommandResult(t *testing.T) {
	commands := &recordingCommandRunner{
		result: outbound.SandboxCommandResult{
			Stdout:   "out\n",
			Stderr:   "err\n",
			ExitCode: 7,
			TimedOut: true,
		},
	}
	runner, err := NewRunner(commands, Config{})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	result := runSandboxExec(t, runner, availableContext(), map[string]string{argumentCommand: "false"})
	if !result.IsError {
		t.Fatal("IsError = false, want true")
	}
	for _, want := range []string{
		"exit_code: 7",
		"timed_out: true",
		"stdout:\nout\n",
		"stderr:\nerr\n",
	} {
		if !strings.Contains(result.Content, want) {
			t.Fatalf("Content = %q, want %q", result.Content, want)
		}
	}
}

func TestRunnerPassesWorkspaceToCommandRunner(t *testing.T) {
	commands := &recordingCommandRunner{}
	runner, err := NewRunner(commands, Config{})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}
	toolContext := availableContext()

	result := runSandboxExec(t, runner, toolContext, map[string]string{argumentCommand: "pwd"})
	if result.IsError {
		t.Fatalf("result = %#v, want success", result)
	}
	if commands.request.Workspace != toolContext.Workspace {
		t.Fatalf("Workspace = %#v, want %#v", commands.request.Workspace, toolContext.Workspace)
	}
	if commands.request.Command != "pwd" {
		t.Fatalf("Command = %q, want pwd", commands.request.Command)
	}
}

func TestRunnerRejectsUnknownToolName(t *testing.T) {
	result, err := newTestRunner(t).RunTool(context.Background(), outbound.ToolCall{
		Name:    "unknown_tool",
		Context: availableContext(),
	})
	if err != nil {
		t.Fatalf("RunTool() error = %v", err)
	}
	if !result.IsError || result.Content != "unknown tool: unknown_tool" {
		t.Fatalf("result = %#v, want unknown tool error", result)
	}
}

func newTestRunner(t *testing.T) *Runner {
	t.Helper()

	runner, err := NewRunner(&recordingCommandRunner{}, Config{})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	return runner
}

func runSandboxExec(
	t *testing.T,
	runner *Runner,
	toolContext outbound.ToolContext,
	arguments map[string]string,
) outbound.ToolResult {
	t.Helper()

	result, err := runner.RunTool(context.Background(), outbound.ToolCall{
		Name:      toolNameSandboxExec,
		Context:   toolContext,
		Arguments: arguments,
	})
	if err != nil {
		t.Fatalf("RunTool() error = %v", err)
	}

	return result
}

func availableContext() outbound.ToolContext {
	return outbound.ToolContext{
		Workspace: testWorkspace(),
		Sandbox:   outbound.Sandbox{ID: "sandbox", SupportsCommands: true},
	}
}

func testWorkspace() outbound.Workspace {
	return outbound.Workspace{
		RootPath:  "/tmp/workspace",
		InputPath: "/tmp/workspace/input",
		WorkPath:  "/tmp/workspace/work",
	}
}

type recordingCommandRunner struct {
	request outbound.SandboxCommandRequest
	result  outbound.SandboxCommandResult
}

func (r *recordingCommandRunner) RunCommand(
	_ context.Context,
	request outbound.SandboxCommandRequest,
) (outbound.SandboxCommandResult, error) {
	r.request = request

	return r.result, nil
}
