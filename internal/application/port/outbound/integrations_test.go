package outbound_test

import (
	"context"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestIntegrationPortContracts(t *testing.T) {
	var ids outbound.IDGenerator = fakeIDGenerator{}
	var sandbox outbound.SandboxProvider = fakeSandboxProvider{}
	var git outbound.GitProvider = fakeGitProvider{}
	var policy outbound.PolicyDecisionPort = fakePolicyDecision{}
	var secrets outbound.SecretBroker = fakeSecretBroker{}

	ctx := context.Background()
	if _, err := ids.NewRunID(); err != nil {
		t.Fatalf("NewRunID() error = %v", err)
	}
	if _, err := sandbox.Prepare(ctx, outbound.SandboxRequest{RunID: "run_test"}); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if err := sandbox.Cleanup(ctx, outbound.Sandbox{ID: "sandbox_test"}); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if _, err := git.Fetch(ctx, outbound.GitFetchRequest{RepositoryURL: "https://example.com/repo.git"}); err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if _, err := policy.Decide(ctx, outbound.PolicyDecisionRequest{RunID: "run_test"}); err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if _, err := secrets.Resolve(ctx, outbound.SecretRequest{ProjectID: "project_test"}); err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
}

type fakeIDGenerator struct{}

func (fakeIDGenerator) NewRunID() (run.RunID, error) {
	return "run_test", nil
}

type fakeSandboxProvider struct{}

func (fakeSandboxProvider) Prepare(context.Context, outbound.SandboxRequest) (outbound.Sandbox, error) {
	return outbound.Sandbox{ID: "sandbox_test"}, nil
}

func (fakeSandboxProvider) Cleanup(context.Context, outbound.Sandbox) error {
	return nil
}

type fakeGitProvider struct{}

func (fakeGitProvider) Fetch(context.Context, outbound.GitFetchRequest) (outbound.GitCheckout, error) {
	return outbound.GitCheckout{Path: "/tmp/repo"}, nil
}

type fakePolicyDecision struct{}

func (fakePolicyDecision) Decide(context.Context, outbound.PolicyDecisionRequest) (outbound.PolicyDecision, error) {
	return outbound.PolicyDecision{Allowed: true}, nil
}

type fakeSecretBroker struct{}

func (fakeSecretBroker) Resolve(context.Context, outbound.SecretRequest) (outbound.SecretBundle, error) {
	return outbound.SecretBundle{Values: map[string]string{}}, nil
}
