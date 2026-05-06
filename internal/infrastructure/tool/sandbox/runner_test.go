package sandbox

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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
	if !strings.Contains(tools[0].Arguments[0].Description, "/workspace") {
		t.Fatalf("command description = %q, want workspace guidance", tools[0].Arguments[0].Description)
	}
	if !strings.Contains(tools[0].Arguments[0].Description, "Stage authorized inputs with workspace") {
		t.Fatalf("command description = %q, want staging guidance", tools[0].Arguments[0].Description)
	}
	if !strings.Contains(tools[0].Description, "general-purpose sandbox") {
		t.Fatalf("description = %q, want general sandbox capability", tools[0].Description)
	}
	if !strings.Contains(tools[0].Arguments[0].Description, "installed sandbox tools and scripts") {
		t.Fatalf("command description = %q, want installed tools guidance", tools[0].Arguments[0].Description)
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

func TestFormatCommandResultKeepsStderrContent(t *testing.T) {
	content := formatCommandResult(outbound.SandboxCommandResult{
		Stderr:   "Syntax Error: bad script\n",
		ExitCode: 1,
	}, 1000)

	if !strings.Contains(content, "Syntax Error: bad script") {
		t.Fatalf("content = %q, want stderr preserved", content)
	}
}

func TestFormatCommandResultPreservesStderrWithLargeStdout(t *testing.T) {
	content := formatCommandResult(outbound.SandboxCommandResult{
		Stdout: strings.Repeat("o", 1000),
		Stderr: strings.Repeat("e", 1000),
	}, 100)

	if !strings.Contains(content, "stdout:\n"+strings.Repeat("o", 50)) {
		t.Fatalf("content = %q, want truncated stdout", content)
	}
	if !strings.Contains(content, "stderr:\n"+strings.Repeat("e", 50)) {
		t.Fatalf("content = %q, want preserved stderr", content)
	}
	if !strings.Contains(content, "truncated: true") {
		t.Fatalf("content = %q, want truncation marker", content)
	}
}

func TestFormatCommandResultPreservesStdoutWithLargeStderr(t *testing.T) {
	content := formatCommandResult(outbound.SandboxCommandResult{
		Stdout: strings.Repeat("o", 1000),
		Stderr: strings.Repeat("e", 1000),
	}, 40)

	if !strings.Contains(content, "stdout:\n"+strings.Repeat("o", 20)) {
		t.Fatalf("content = %q, want preserved stdout", content)
	}
	if !strings.Contains(content, "stderr:\n"+strings.Repeat("e", 20)) {
		t.Fatalf("content = %q, want truncated stderr", content)
	}
	if !strings.Contains(content, "truncated: true") {
		t.Fatalf("content = %q, want truncation marker", content)
	}
}

func TestFormatCommandResultStdoutOnlyUsesWholeBudget(t *testing.T) {
	content := formatCommandResult(outbound.SandboxCommandResult{
		Stdout: "abcdefghijklmnop",
	}, 8)

	if !strings.Contains(content, "stdout:\nabcdefgh\n") {
		t.Fatalf("content = %q, want stdout to use whole budget", content)
	}
	if !strings.Contains(content, "truncated: true") {
		t.Fatalf("content = %q, want truncation marker", content)
	}
}

func TestFormatCommandResultStderrOnlyUsesWholeBudget(t *testing.T) {
	content := formatCommandResult(outbound.SandboxCommandResult{
		Stderr: "abcdefghijklmnop",
	}, 8)

	if !strings.Contains(content, "stderr:\nabcdefgh\n") {
		t.Fatalf("content = %q, want stderr to use whole budget", content)
	}
	if !strings.Contains(content, "truncated: true") {
		t.Fatalf("content = %q, want truncation marker", content)
	}
}

func TestFormatCommandResultTruncatesUTF8Safely(t *testing.T) {
	content := formatCommandResult(outbound.SandboxCommandResult{
		Stdout: strings.Repeat("界", 10),
		Stderr: strings.Repeat("錯", 10),
	}, 10)

	if !utf8.ValidString(content) {
		t.Fatalf("content is not valid UTF-8: %q", content)
	}
	if !strings.Contains(content, "stdout:\n界\n") {
		t.Fatalf("content = %q, want safe stdout rune", content)
	}
	if !strings.Contains(content, "stderr:\n錯\n") {
		t.Fatalf("content = %q, want safe stderr rune", content)
	}
	if !strings.Contains(content, "truncated: true") {
		t.Fatalf("content = %q, want truncation marker", content)
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
	if commands.request.Workspace.RootPath != toolContext.Workspace.RootPath {
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
		RootPath: "/tmp/workspace",
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
