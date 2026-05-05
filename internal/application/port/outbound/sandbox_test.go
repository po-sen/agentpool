package outbound

import (
	"context"
	"testing"
)

func TestSandboxProviderContract(t *testing.T) {
	var provider SandboxProvider = contractSandboxProvider{}
	var capabilities SandboxCapabilityProvider = contractSandboxProvider{}

	ctx := context.Background()
	sandbox, err := provider.Prepare(ctx, SandboxRequest{RunID: "run_test"})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if sandbox.ID != "sandbox_test" {
		t.Fatalf("sandbox ID = %q, want sandbox_test", sandbox.ID)
	}
	if err := provider.Cleanup(ctx, sandbox); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if !capabilities.Capabilities().SupportsCommands {
		t.Fatal("SupportsCommands = false, want true")
	}
}

type contractSandboxProvider struct{}

func (contractSandboxProvider) Prepare(context.Context, SandboxRequest) (Sandbox, error) {
	return Sandbox{ID: "sandbox_test", SupportsCommands: true}, nil
}

func (contractSandboxProvider) Cleanup(context.Context, Sandbox) error {
	return nil
}

func (contractSandboxProvider) Capabilities() SandboxCapabilities {
	return SandboxCapabilities{SupportsCommands: true}
}
