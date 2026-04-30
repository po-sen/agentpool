package sandbox

import (
	"context"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

// NoopProvider returns a placeholder sandbox.
type NoopProvider struct{}

var _ outbound.SandboxProvider = (*NoopProvider)(nil)

// NewNoopProvider creates a no-op sandbox provider.
func NewNoopProvider() *NoopProvider {
	return &NoopProvider{}
}

// Prepare returns a placeholder sandbox.
func (p *NoopProvider) Prepare(context.Context, outbound.SandboxRequest) (outbound.Sandbox, error) {
	return outbound.Sandbox{ID: "noop"}, nil
}

// Cleanup records no side effects.
func (p *NoopProvider) Cleanup(context.Context, outbound.Sandbox) error {
	return nil
}
