package noop

import (
	"context"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

// Provider returns a placeholder sandbox.
type Provider struct{}

var _ outbound.SandboxProvider = (*Provider)(nil)
var _ outbound.SandboxCapabilityProvider = (*Provider)(nil)
var _ outbound.SandboxCommandRunner = (*Provider)(nil)

// NewProvider creates a no-op sandbox provider.
func NewProvider() *Provider {
	return &Provider{}
}

// Prepare returns a placeholder sandbox.
func (p *Provider) Prepare(context.Context, outbound.SandboxRequest) (outbound.Sandbox, error) {
	return outbound.Sandbox{ID: "noop"}, nil
}

// Capabilities reports that the no-op sandbox cannot execute commands.
func (p *Provider) Capabilities() outbound.SandboxCapabilities {
	return outbound.SandboxCapabilities{}
}

// Cleanup records no side effects.
func (p *Provider) Cleanup(context.Context, outbound.Sandbox) error {
	return nil
}

// RunCommand reports that the no-op sandbox cannot execute commands.
func (p *Provider) RunCommand(context.Context, outbound.SandboxCommandRequest) (outbound.SandboxCommandResult, error) {
	return outbound.SandboxCommandResult{
		Stderr:   "sandbox command execution is not available",
		ExitCode: 1,
	}, nil
}
