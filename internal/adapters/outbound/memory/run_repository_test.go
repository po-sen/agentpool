package memory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/po-sen/agentpool/internal/adapters/outbound/memory"
	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestRunRepositoryListReturnsDetachedCopies(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRunRepository()
	now := time.Unix(100, 0).UTC()

	item, err := run.New("run_test", run.TaskSpec{Prompt: "do work"}, now)
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	if err := repo.Save(ctx, item); err != nil {
		t.Fatalf("save run: %v", err)
	}

	items, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}

	items[0].Status = run.StatusCompleted

	items, err = repo.List(ctx)
	if err != nil {
		t.Fatalf("list runs again: %v", err)
	}
	if items[0].Status != run.StatusQueued {
		t.Fatalf("stored status = %s, want %s", items[0].Status, run.StatusQueued)
	}
}

func TestRunRepositoryFindByIDReturnsNotFound(t *testing.T) {
	repo := memory.NewRunRepository()

	_, err := repo.FindByID(context.Background(), "run_missing")
	if !errors.Is(err, outbound.ErrRunNotFound) {
		t.Fatalf("find by ID error = %v, want %v", err, outbound.ErrRunNotFound)
	}
}

func TestRunRepositoryListOrdersByCreatedAtThenID(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRunRepository()
	firstTime := time.Unix(100, 0).UTC()
	secondTime := time.Unix(200, 0).UTC()

	saveRun(ctx, t, repo, "run_b", firstTime)
	saveRun(ctx, t, repo, "run_c", secondTime)
	saveRun(ctx, t, repo, "run_a", firstTime)

	items, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("len(items) = %d, want 3", len(items))
	}

	got := []run.RunID{items[0].ID, items[1].ID, items[2].ID}
	want := []run.RunID{"run_a", "run_b", "run_c"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("items[%d].ID = %s, want %s; got order %v", i, got[i], want[i], got)
		}
	}
}

func saveRun(ctx context.Context, t *testing.T, repo *memory.RunRepository, id run.RunID, createdAt time.Time) {
	t.Helper()

	item, err := run.New(id, run.TaskSpec{Prompt: "do work"}, createdAt)
	if err != nil {
		t.Fatalf("new run %s: %v", id, err)
	}
	if err := repo.Save(ctx, item); err != nil {
		t.Fatalf("save run %s: %v", id, err)
	}
}
