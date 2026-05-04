package noop

import (
	"context"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

// Provider resolves only the absence of workspace access.
type Provider struct{}

var _ outbound.WorkspaceProvider = (*Provider)(nil)

// NewProvider creates a no-op workspace provider.
func NewProvider() *Provider {
	return &Provider{}
}

// ResolveWorkspace returns no workspace for supported none sources.
func (p *Provider) ResolveWorkspace(_ context.Context, request outbound.WorkspaceResolveRequest) (outbound.Workspace, error) {
	switch request.Source.EffectiveType() {
	case run.WorkspaceSourceNone:
		return outbound.Workspace{}, nil
	default:
		return outbound.Workspace{}, run.ErrUnknownWorkspaceSource
	}
}

// CleanupWorkspace records no side effects.
func (p *Provider) CleanupWorkspace(context.Context, outbound.Workspace) error {
	return nil
}
