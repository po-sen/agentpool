package docker

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestProviderCapabilitiesSupportCommands(t *testing.T) {
	provider := NewProvider(Config{})

	capabilities := provider.Capabilities()
	if !capabilities.SupportsCommands {
		t.Fatal("SupportsCommands = false, want true")
	}
}

func TestProviderPrepareReturnsCommandCapableSandbox(t *testing.T) {
	provider := NewProvider(Config{})

	sandbox, err := provider.Prepare(context.Background(), outbound.SandboxRequest{
		RunID:     "run_test",
		Workspace: testWorkspace(),
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if sandbox.ID != sandboxID {
		t.Fatalf("Sandbox.ID = %q, want %q", sandbox.ID, sandboxID)
	}
	if !sandbox.SupportsCommands {
		t.Fatal("Sandbox.SupportsCommands = false, want true")
	}
}

func TestProviderRunCommandBuildsDockerRun(t *testing.T) {
	executor := &recordingExecutor{
		output: commandOutput{
			stdout: "ok\n",
		},
	}
	provider := newProviderWithExecutor(Config{Image: "busybox:1.36"}, executor)

	result, err := provider.RunCommand(context.Background(), outbound.SandboxCommandRequest{
		Sandbox:   commandCapableSandbox(),
		Workspace: testWorkspace(),
		Command:   "pwd",
	})
	if err != nil {
		t.Fatalf("RunCommand() error = %v", err)
	}
	if result.Stdout != "ok\n" {
		t.Fatalf("Stdout = %q, want ok", result.Stdout)
	}
	if executor.name != dockerBinary {
		t.Fatalf("executor name = %q, want %q", executor.name, dockerBinary)
	}

	wantArgs := []string{
		"run",
		"--rm",
		"--network",
		"none",
		"-v",
		"/tmp/workspace:/workspace:rw",
		"-w",
		"/workspace",
		"busybox:1.36",
		"/bin/sh",
		"-lc",
		"pwd",
	}
	if !reflect.DeepEqual(executor.args, wantArgs) {
		t.Fatalf("docker args = %#v, want %#v", executor.args, wantArgs)
	}
}

func TestProviderRunCommandMapsNonZeroExitCode(t *testing.T) {
	provider := newProviderWithExecutor(Config{}, &recordingExecutor{
		output: commandOutput{
			stderr:   "failed\n",
			exitCode: 7,
			err:      errors.New("exit status 7"),
		},
	})

	result, err := provider.RunCommand(context.Background(), outbound.SandboxCommandRequest{
		Sandbox:   commandCapableSandbox(),
		Workspace: testWorkspace(),
		Command:   "false",
	})
	if err != nil {
		t.Fatalf("RunCommand() error = %v", err)
	}
	if result.ExitCode != 7 {
		t.Fatalf("ExitCode = %d, want 7", result.ExitCode)
	}
	if result.Stderr != "failed\n" {
		t.Fatalf("Stderr = %q, want failed", result.Stderr)
	}
}

func TestProviderRunCommandMapsTimeout(t *testing.T) {
	provider := newProviderWithExecutor(Config{}, &recordingExecutor{
		output: commandOutput{
			stderr:   "killed\n",
			exitCode: -1,
			timedOut: true,
			err:      context.DeadlineExceeded,
		},
	})

	result, err := provider.RunCommand(context.Background(), outbound.SandboxCommandRequest{
		Sandbox:   commandCapableSandbox(),
		Workspace: testWorkspace(),
		Command:   "sleep 60",
	})
	if err != nil {
		t.Fatalf("RunCommand() error = %v", err)
	}
	if !result.TimedOut {
		t.Fatal("TimedOut = false, want true")
	}
	if result.ExitCode != -1 {
		t.Fatalf("ExitCode = %d, want -1", result.ExitCode)
	}
}

func TestProviderRunCommandReportsDockerStartFailure(t *testing.T) {
	provider := newProviderWithExecutor(Config{}, &recordingExecutor{
		output: commandOutput{
			err: errors.New("executable file not found"),
		},
	})

	_, err := provider.RunCommand(context.Background(), outbound.SandboxCommandRequest{
		Sandbox:   commandCapableSandbox(),
		Workspace: testWorkspace(),
		Command:   "pwd",
	})
	if err == nil {
		t.Fatal("RunCommand() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "docker CLI execution failed") {
		t.Fatalf("RunCommand() error = %v, want docker CLI failure", err)
	}
}

func TestProviderCleanupIsNoop(t *testing.T) {
	provider := NewProvider(Config{})

	if err := provider.Cleanup(context.Background(), commandCapableSandbox()); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
}

func TestProviderRunCommandRejectsMissingWorkspaceRootPath(t *testing.T) {
	provider := newProviderWithExecutor(Config{}, &recordingExecutor{})

	_, err := provider.RunCommand(context.Background(), outbound.SandboxCommandRequest{
		Sandbox: commandCapableSandbox(),
		Command: "pwd",
	})
	if err == nil || !strings.Contains(err.Error(), "workspace root path is required") {
		t.Fatalf("RunCommand() error = %v, want missing workspace root path", err)
	}
}

func TestProviderRunCommandRejectsMissingCommand(t *testing.T) {
	provider := newProviderWithExecutor(Config{}, &recordingExecutor{})

	_, err := provider.RunCommand(context.Background(), outbound.SandboxCommandRequest{
		Sandbox:   commandCapableSandbox(),
		Workspace: testWorkspace(),
	})
	if err == nil || !strings.Contains(err.Error(), "command is required") {
		t.Fatalf("RunCommand() error = %v, want missing command", err)
	}
}

func TestProviderPrepareRejectsMissingWorkspace(t *testing.T) {
	provider := NewProvider(Config{})

	_, err := provider.Prepare(context.Background(), outbound.SandboxRequest{RunID: "run_test"})
	if err == nil || !strings.Contains(err.Error(), "workspace root path is required") {
		t.Fatalf("Prepare() error = %v, want missing workspace path", err)
	}
}

func commandCapableSandbox() outbound.Sandbox {
	return outbound.Sandbox{ID: sandboxID, SupportsCommands: true}
}

func testWorkspace() outbound.Workspace {
	return outbound.Workspace{
		RootPath: "/tmp/workspace",
	}
}

type recordingExecutor struct {
	name   string
	args   []string
	output commandOutput
}

func (e *recordingExecutor) Run(_ context.Context, name string, args ...string) commandOutput {
	e.name = name
	e.args = append([]string(nil), args...)

	return e.output
}
