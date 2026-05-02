package noop

import (
	"context"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

// Broker returns an empty secret bundle.
type Broker struct{}

var _ outbound.SecretBroker = (*Broker)(nil)

// NewBroker creates a no-op secret broker.
func NewBroker() *Broker {
	return &Broker{}
}

// Resolve returns an empty secret bundle.
func (b *Broker) Resolve(context.Context, outbound.SecretRequest) (outbound.SecretBundle, error) {
	return outbound.SecretBundle{Values: map[string]string{}}, nil
}
