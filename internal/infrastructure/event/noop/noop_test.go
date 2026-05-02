package noop_test

import (
	"context"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	eventnoop "github.com/po-sen/agentpool/internal/infrastructure/event/noop"
)

func TestPublisherPublishAcceptsEvent(t *testing.T) {
	publisher := eventnoop.NewPublisher()

	err := publisher.Publish(context.Background(), outbound.Event{
		Type:  outbound.EventRunCreated,
		RunID: "run_test",
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
}
