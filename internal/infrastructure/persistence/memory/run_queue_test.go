package memory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/infrastructure/persistence/memory"
)

func TestRunQueueDequeuesInOrderAndReportsEmpty(t *testing.T) {
	ctx := context.Background()
	queue := memory.NewRunQueue()

	_, err := queue.Dequeue(ctx)
	if !errors.Is(err, outbound.ErrRunQueueEmpty) {
		t.Fatalf("empty dequeue error = %v, want %v", err, outbound.ErrRunQueueEmpty)
	}

	if err := queue.Enqueue(ctx, "run_a"); err != nil {
		t.Fatalf("enqueue run_a: %v", err)
	}
	if err := queue.Enqueue(ctx, "run_b"); err != nil {
		t.Fatalf("enqueue run_b: %v", err)
	}

	first, err := queue.Dequeue(ctx)
	if err != nil {
		t.Fatalf("first dequeue: %v", err)
	}
	if first != "run_a" {
		t.Fatalf("first dequeue = %s, want run_a", first)
	}

	second, err := queue.Dequeue(ctx)
	if err != nil {
		t.Fatalf("second dequeue: %v", err)
	}
	if second != "run_b" {
		t.Fatalf("second dequeue = %s, want run_b", second)
	}
}

func TestRunQueueDequeueHonorsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := memory.NewRunQueue().Dequeue(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("dequeue error = %v, want %v", err, context.Canceled)
	}
}
