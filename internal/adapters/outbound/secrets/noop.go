package secrets

import (
	"context"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

// NoopBroker returns an empty secret bundle.
type NoopBroker struct{}

var _ outbound.SecretBroker = (*NoopBroker)(nil)

// NewNoopBroker creates a no-op secret broker.
func NewNoopBroker() *NoopBroker {
	return &NoopBroker{}
}

// Resolve returns an empty secret bundle.
func (b *NoopBroker) Resolve(context.Context, outbound.SecretRequest) (outbound.SecretBundle, error) {
	return outbound.SecretBundle{Values: map[string]string{}}, nil
}
