package git

import (
	"context"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

// NoopProvider returns an empty checkout placeholder.
type NoopProvider struct{}

var _ outbound.GitProvider = (*NoopProvider)(nil)

// NewNoopProvider creates a no-op git provider.
func NewNoopProvider() *NoopProvider {
	return &NoopProvider{}
}

// Fetch returns an empty checkout placeholder.
func (p *NoopProvider) Fetch(context.Context, outbound.GitFetchRequest) (outbound.GitCheckout, error) {
	return outbound.GitCheckout{}, nil
}
