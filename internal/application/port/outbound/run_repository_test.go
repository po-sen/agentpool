package outbound_test

import (
	"context"
	"testing"
	"time"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestRunRepositoryContract(t *testing.T) {
	var repo outbound.RunRepository = newFakeRunRepository()
	ctx := context.Background()

	item, err := run.New("run_test", run.TaskSpec{Prompt: "do work"}, time.Unix(100, 0).UTC())
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	if err := repo.Save(ctx, item); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if ok, err := repo.SaveIfStatus(ctx, item, run.StatusQueued); err != nil || !ok {
		t.Fatalf("SaveIfStatus() = (%v, %v), want (true, nil)", ok, err)
	}

	found, err := repo.FindByID(ctx, item.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if found.ID != item.ID {
		t.Fatalf("found ID = %s, want %s", found.ID, item.ID)
	}

	items, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
}

type fakeRunRepository struct {
	item *run.Run
}

func newFakeRunRepository() *fakeRunRepository {
	return &fakeRunRepository{}
}

func (r *fakeRunRepository) Save(_ context.Context, item *run.Run) error {
	r.item = item

	return nil
}

func (r *fakeRunRepository) SaveIfStatus(_ context.Context, item *run.Run, expected run.Status) (bool, error) {
	if r.item == nil || r.item.Status != expected {
		return false, nil
	}

	r.item = item

	return true, nil
}

func (r *fakeRunRepository) FindByID(context.Context, run.RunID) (*run.Run, error) {
	return r.item, nil
}

func (r *fakeRunRepository) List(context.Context) ([]*run.Run, error) {
	return []*run.Run{r.item}, nil
}
