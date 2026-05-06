package outbound

import (
	"context"
	"testing"
)

func TestSandboxCommandRunnerContract(t *testing.T) {
	var runner SandboxCommandRunner = contractSandboxCommandRunner{}

	result, err := runner.RunCommand(context.Background(), SandboxCommandRequest{
		Sandbox:   Sandbox{ID: "sandbox_test"},
		Workspace: Workspace{RootPath: "/tmp/workspace"},
		Command:   "true",
	})
	if err != nil {
		t.Fatalf("RunCommand() error = %v", err)
	}
	if result.Stdout != "ok" {
		t.Fatalf("Stdout = %q, want ok", result.Stdout)
	}
}

type contractSandboxCommandRunner struct{}

func (contractSandboxCommandRunner) RunCommand(context.Context, SandboxCommandRequest) (SandboxCommandResult, error) {
	return SandboxCommandResult{Stdout: "ok"}, nil
}
