package noop

import (
	"context"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestPublisherPublishAcceptsEvent(t *testing.T) {
	publisher := NewPublisher()

	err := publisher.Publish(context.Background(), outbound.Event{
		Type:  outbound.EventRunCreated,
		RunID: "run_test",
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
}
