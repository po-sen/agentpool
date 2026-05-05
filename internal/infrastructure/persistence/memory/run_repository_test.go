package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestRunRepositoryListReturnsDetachedCopies(t *testing.T) {
	ctx := context.Background()
	repo := NewRunRepository()
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

func TestRunRepositoryStoresArtifactContentInMemory(t *testing.T) {
	ctx := context.Background()
	repo := NewRunRepository()
	item, err := run.New("run_test", run.TaskSpec{Prompt: "do work"}, time.Unix(100, 0).UTC())
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	item.RecordArtifacts(time.Unix(101, 0).UTC(), []run.Artifact{
		{Path: "report.md", Content: []byte("# Report\n"), SizeBytes: 9},
	})
	if err := repo.Save(ctx, item); err != nil {
		t.Fatalf("save run: %v", err)
	}

	found, err := repo.FindByID(ctx, "run_test")
	if err != nil {
		t.Fatalf("find run: %v", err)
	}
	if string(found.Artifacts[0].Content) != "# Report\n" {
		t.Fatalf("artifact content = %q, want report", string(found.Artifacts[0].Content))
	}
	found.Artifacts[0].Content[0] = 'X'

	found, err = repo.FindByID(ctx, "run_test")
	if err != nil {
		t.Fatalf("find run again: %v", err)
	}
	if string(found.Artifacts[0].Content) != "# Report\n" {
		t.Fatalf("artifact content after mutation = %q, want detached content", string(found.Artifacts[0].Content))
	}
}

func TestRunRepositoryFindByIDReturnsNotFound(t *testing.T) {
	repo := NewRunRepository()

	_, err := repo.FindByID(context.Background(), "run_missing")
	if !errors.Is(err, run.ErrRunNotFound) {
		t.Fatalf("find by ID error = %v, want %v", err, run.ErrRunNotFound)
	}
}

func TestRunRepositorySaveIfStatusUpdatesOnlyExpectedStatus(t *testing.T) {
	ctx := context.Background()
	repo := NewRunRepository()
	now := time.Unix(100, 0).UTC()

	item, err := run.New("run_test", run.TaskSpec{Prompt: "do work"}, now)
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	if err := repo.Save(ctx, item); err != nil {
		t.Fatalf("save run: %v", err)
	}

	item.Status = run.StatusRunning
	saved, err := repo.SaveIfStatus(ctx, item, run.StatusQueued)
	if err != nil {
		t.Fatalf("save if queued: %v", err)
	}
	if !saved {
		t.Fatal("save if queued = false, want true")
	}

	item.Status = run.StatusCompleted
	saved, err = repo.SaveIfStatus(ctx, item, run.StatusQueued)
	if err != nil {
		t.Fatalf("save if queued again: %v", err)
	}
	if saved {
		t.Fatal("save if queued after status changed = true, want false")
	}

	stored, err := repo.FindByID(ctx, item.ID)
	if err != nil {
		t.Fatalf("find stored run: %v", err)
	}
	if stored.Status != run.StatusRunning {
		t.Fatalf("stored status = %s, want %s", stored.Status, run.StatusRunning)
	}
}

func TestRunRepositoryListOrdersByCreatedAtThenID(t *testing.T) {
	ctx := context.Background()
	repo := NewRunRepository()
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

func saveRun(ctx context.Context, t *testing.T, repo *RunRepository, id run.RunID, createdAt time.Time) {
	t.Helper()

	item, err := run.New(id, run.TaskSpec{Prompt: "do work"}, createdAt)
	if err != nil {
		t.Fatalf("new run %s: %v", id, err)
	}
	if err := repo.Save(ctx, item); err != nil {
		t.Fatalf("save run %s: %v", id, err)
	}
}
