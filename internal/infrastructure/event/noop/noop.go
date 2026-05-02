package noop

import (
	"context"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

// Publisher accepts events without sending them anywhere.
type Publisher struct{}

var _ outbound.EventPublisher = (*Publisher)(nil)

// NewPublisher creates a no-op event publisher.
func NewPublisher() *Publisher {
	return &Publisher{}
}

// Publish records no side effects.
func (p *Publisher) Publish(context.Context, outbound.Event) error {
	return nil
}
