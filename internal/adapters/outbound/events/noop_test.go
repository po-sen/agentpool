package events_test

import (
	"context"
	"testing"

	"github.com/po-sen/agentpool/internal/adapters/outbound/events"
	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestNoopPublisherPublishAcceptsEvent(t *testing.T) {
	publisher := events.NewNoopPublisher()

	err := publisher.Publish(context.Background(), outbound.Event{
		Type:  outbound.EventRunCreated,
		RunID: "run_test",
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
}
