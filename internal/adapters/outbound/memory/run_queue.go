package memory

import (
	"context"
	"sync"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

// RunQueue stores queued run IDs in process memory.
type RunQueue struct {
	mu    sync.Mutex
	items []run.RunID
}

var _ outbound.RunQueue = (*RunQueue)(nil)

// NewRunQueue creates an empty in-memory run queue.
func NewRunQueue() *RunQueue {
	return &RunQueue{}
}

// Enqueue appends a run ID to the queue.
func (q *RunQueue) Enqueue(_ context.Context, id run.RunID) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.items = append(q.items, id)

	return nil
}

// Dequeue removes and returns the next queued run ID.
func (q *RunQueue) Dequeue(ctx context.Context) (run.RunID, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) == 0 {
		return "", outbound.ErrRunQueueEmpty
	}

	id := q.items[0]
	q.items = q.items[1:]

	return id, nil
}
