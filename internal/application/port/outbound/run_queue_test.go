package outbound_test

import (
	"context"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestRunQueueContract(t *testing.T) {
	var queue outbound.RunQueue = &fakeRunQueue{}

	ctx := context.Background()
	if err := queue.Enqueue(ctx, "run_test"); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	id, err := queue.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue() error = %v", err)
	}
	if id != "run_test" {
		t.Fatalf("Dequeue() = %s, want run_test", id)
	}
}

type fakeRunQueue struct {
	id run.RunID
}

func (q *fakeRunQueue) Enqueue(_ context.Context, id run.RunID) error {
	q.id = id

	return nil
}

func (q *fakeRunQueue) Dequeue(context.Context) (run.RunID, error) {
	return q.id, nil
}
