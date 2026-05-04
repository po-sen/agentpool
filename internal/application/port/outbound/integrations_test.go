package outbound

import (
	"context"
	"testing"

	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestIntegrationPortContracts(t *testing.T) {
	var ids IDGenerator = fakeIDGenerator{}
	var sandbox SandboxProvider = fakeSandboxProvider{}
	var sandboxCommands SandboxCommandRunner = fakeSandboxCommandRunner{}
	var git GitProvider = fakeGitProvider{}
	var policy PolicyDecisionPort = fakePolicyDecision{}
	var secrets SecretBroker = fakeSecretBroker{}
	var tools ToolRunner = fakeToolRunner{}

	ctx := context.Background()
	if _, err := ids.NewRunID(); err != nil {
		t.Fatalf("NewRunID() error = %v", err)
	}
	if _, err := sandbox.Prepare(ctx, SandboxRequest{RunID: "run_test"}); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if err := sandbox.Cleanup(ctx, Sandbox{ID: "sandbox_test"}); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if _, err := sandboxCommands.RunCommand(ctx, SandboxCommandRequest{
		Sandbox: Sandbox{ID: "sandbox_test"},
		Command: "true",
	}); err != nil {
		t.Fatalf("RunCommand() error = %v", err)
	}
	if _, err := git.Fetch(ctx, GitFetchRequest{RepositoryURL: "https://example.com/repo.git"}); err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if _, err := policy.Decide(ctx, PolicyDecisionRequest{RunID: "run_test"}); err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if _, err := secrets.Resolve(ctx, SecretRequest{ProjectID: "project_test"}); err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if _, err := tools.ListTools(ctx, ToolListRequest{RunID: "run_test"}); err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if _, err := tools.RunTool(ctx, ToolCall{RunID: "run_test", Name: "echo"}); err != nil {
		t.Fatalf("RunTool() error = %v", err)
	}
}

type fakeIDGenerator struct{}

func (fakeIDGenerator) NewRunID() (run.RunID, error) {
	return "run_test", nil
}

type fakeSandboxProvider struct{}

func (fakeSandboxProvider) Prepare(context.Context, SandboxRequest) (Sandbox, error) {
	return Sandbox{ID: "sandbox_test", SupportsCommands: true}, nil
}

func (fakeSandboxProvider) Cleanup(context.Context, Sandbox) error {
	return nil
}

func (fakeSandboxProvider) Capabilities() SandboxCapabilities {
	return SandboxCapabilities{SupportsCommands: true}
}

type fakeSandboxCommandRunner struct{}

func (fakeSandboxCommandRunner) RunCommand(context.Context, SandboxCommandRequest) (SandboxCommandResult, error) {
	return SandboxCommandResult{Stdout: "ok"}, nil
}

type fakeGitProvider struct{}

func (fakeGitProvider) Fetch(context.Context, GitFetchRequest) (GitCheckout, error) {
	return GitCheckout{Path: "/tmp/repo"}, nil
}

type fakePolicyDecision struct{}

func (fakePolicyDecision) Decide(context.Context, PolicyDecisionRequest) (PolicyDecision, error) {
	return PolicyDecision{Allowed: true}, nil
}

type fakeSecretBroker struct{}

func (fakeSecretBroker) Resolve(context.Context, SecretRequest) (SecretBundle, error) {
	return SecretBundle{Values: map[string]string{}}, nil
}

type fakeToolRunner struct{}

func (fakeToolRunner) ListTools(context.Context, ToolListRequest) ([]ToolDefinition, error) {
	return []ToolDefinition{{Name: "echo", Description: "test tool"}}, nil
}

func (fakeToolRunner) RunTool(context.Context, ToolCall) (ToolResult, error) {
	return ToolResult{Content: "done"}, nil
}
