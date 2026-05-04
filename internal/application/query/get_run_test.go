package query

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/po-sen/agentpool/internal/application/port/inbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestGetRunReturnsRunByID(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	item := saveRun(ctx, t, repo, "run_test", "do work", time.Unix(100, 0).UTC())
	handler := NewGetRunHandler(repo)

	found, err := handler.GetRun(ctx, inbound.GetRunQuery{RunID: item.ID.String()})
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if found.ID != item.ID.String() {
		t.Fatalf("found ID = %s, want %s", found.ID, item.ID)
	}
	if found.Task.Prompt != item.Task.Prompt {
		t.Fatalf("found prompt = %q, want %q", found.Task.Prompt, item.Task.Prompt)
	}
}

func TestGetRunReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	handler := NewGetRunHandler(newFakeRunRepository())

	_, err := handler.GetRun(ctx, inbound.GetRunQuery{RunID: "run_missing"})
	if !errors.Is(err, inbound.ErrRunNotFound) {
		t.Fatalf("GetRun() error = %v, want %v", err, inbound.ErrRunNotFound)
	}
}

type runRepository interface {
	Save(context.Context, *run.Run) error
}

type fakeRunRepository struct {
	items []run.RunID
	runs  map[run.RunID]*run.Run
}

func newFakeRunRepository() *fakeRunRepository {
	return &fakeRunRepository{
		runs: make(map[run.RunID]*run.Run),
	}
}

func (r *fakeRunRepository) Save(_ context.Context, item *run.Run) error {
	if _, ok := r.runs[item.ID]; !ok {
		r.items = append(r.items, item.ID)
	}
	r.runs[item.ID] = item.Clone()

	return nil
}

func (r *fakeRunRepository) FindByID(_ context.Context, id run.RunID) (*run.Run, error) {
	item, ok := r.runs[id]
	if !ok {
		return nil, run.ErrRunNotFound
	}

	return item.Clone(), nil
}

func (r *fakeRunRepository) List(context.Context) ([]*run.Run, error) {
	items := make([]*run.Run, 0, len(r.items))
	for _, id := range r.items {
		items = append(items, r.runs[id].Clone())
	}

	return items, nil
}

func saveRun(
	ctx context.Context,
	t *testing.T,
	repo runRepository,
	id run.RunID,
	prompt string,
	now time.Time,
) *run.Run {
	t.Helper()

	item, err := run.New(id, run.TaskSpec{Prompt: prompt}, now)
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	if err := repo.Save(ctx, item); err != nil {
		t.Fatalf("save run: %v", err)
	}

	return item
}
