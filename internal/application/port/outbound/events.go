package outbound

import (
	"context"
	"time"

	"github.com/po-sen/agentpool/internal/domain/run"
)

const (
	// EventRunCreated is emitted after a run is accepted and queued.
	EventRunCreated = "run.created"
	// EventRunPreparing is emitted when a worker starts preparing a run.
	EventRunPreparing = "run.preparing"
	// EventRunStarted is emitted when a worker starts executing a run.
	EventRunStarted = "run.started"
	// EventRunCompleted is emitted when a run completes successfully.
	EventRunCompleted = "run.completed"
	// EventRunFailed is emitted when a run fails.
	EventRunFailed = "run.failed"
	// EventRunCancelled is emitted after a run is cancelled.
	EventRunCancelled = "run.cancelled"
)

// Event is a small application event published to interested consumers.
type Event struct {
	Type       string
	RunID      run.RunID
	OccurredAt time.Time
	Attributes map[string]string
}

// EventPublisher publishes run lifecycle events.
type EventPublisher interface {
	Publish(context.Context, Event) error
}
