package noop

import (
	"context"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

// Provider returns a placeholder sandbox.
type Provider struct{}

var _ outbound.SandboxProvider = (*Provider)(nil)

// NewProvider creates a no-op sandbox provider.
func NewProvider() *Provider {
	return &Provider{}
}

// Prepare returns a placeholder sandbox.
func (p *Provider) Prepare(context.Context, outbound.SandboxRequest) (outbound.Sandbox, error) {
	return outbound.Sandbox{ID: "noop"}, nil
}

// Cleanup records no side effects.
func (p *Provider) Cleanup(context.Context, outbound.Sandbox) error {
	return nil
}
