package query

import (
	"context"
	"testing"
	"time"

	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestListRunsReturnsCreatedRuns(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	first := saveRun(ctx, t, repo, "run_1", "first", time.Unix(100, 0).UTC())
	second := saveRun(ctx, t, repo, "run_2", "second", time.Unix(101, 0).UTC())
	handler := NewListRunsHandler(repo)

	items, err := handler.ListRuns(ctx)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].ID != first.ID.String() || items[1].ID != second.ID.String() {
		t.Fatalf("listed IDs = [%s %s], want [%s %s]", items[0].ID, items[1].ID, first.ID, second.ID)
	}
}

func TestListRunsReturnsDetachedCopies(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	item := saveRun(ctx, t, repo, "run_test", "do work", time.Unix(100, 0).UTC())
	handler := NewListRunsHandler(repo)

	items, err := handler.ListRuns(ctx)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}

	items[0].Status = string(run.StatusCompleted)
	items[0].Task.Prompt = "mutated"

	items, err = handler.ListRuns(ctx)
	if err != nil {
		t.Fatalf("list runs again: %v", err)
	}
	if items[0].Status != string(run.StatusQueued) {
		t.Fatalf("listed status = %s, want %s", items[0].Status, run.StatusQueued)
	}
	if items[0].Task.Prompt != item.Task.Prompt {
		t.Fatalf("listed prompt = %q, want %q", items[0].Task.Prompt, item.Task.Prompt)
	}
}
