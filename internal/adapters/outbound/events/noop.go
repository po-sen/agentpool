package events

import (
	"context"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

// NoopPublisher accepts events without sending them anywhere.
type NoopPublisher struct{}

var _ outbound.EventPublisher = (*NoopPublisher)(nil)

// NewNoopPublisher creates a no-op event publisher.
func NewNoopPublisher() *NoopPublisher {
	return &NoopPublisher{}
}

// Publish records no side effects.
func (p *NoopPublisher) Publish(context.Context, outbound.Event) error {
	return nil
}
