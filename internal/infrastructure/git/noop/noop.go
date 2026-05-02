package noop

import (
	"context"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

// Provider returns an empty checkout placeholder.
type Provider struct{}

var _ outbound.GitProvider = (*Provider)(nil)

// NewProvider creates a no-op git provider.
func NewProvider() *Provider {
	return &Provider{}
}

// Fetch returns an empty checkout placeholder.
func (p *Provider) Fetch(context.Context, outbound.GitFetchRequest) (outbound.GitCheckout, error) {
	return outbound.GitCheckout{}, nil
}
