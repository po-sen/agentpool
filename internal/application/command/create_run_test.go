package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/po-sen/agentpool/internal/application/port/inbound"
	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestCreateRunCreatesQueuedRun(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	queue := &fakeRunQueue{}
	publisher := &recordingPublisher{}
	now := time.Unix(100, 0).UTC()

	handler := NewCreateRunHandler(
		repo,
		queue,
		publisher,
		fixedIDGenerator{id: "run_test"},
		WithCreateRunClock(func() time.Time { return now }),
	)

	created, err := handler.CreateRun(ctx, inbound.CreateRunCommand{
		ProjectID: "project_test",
		Prompt:    "do work",
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	if created.ID != "run_test" {
		t.Fatalf("created ID = %s, want run_test", created.ID)
	}
	if created.Status != string(run.StatusQueued) {
		t.Fatalf("created status = %s, want %s", created.Status, run.StatusQueued)
	}

	stored, err := repo.FindByID(ctx, run.RunID(created.ID))
	if err != nil {
		t.Fatalf("find stored run: %v", err)
	}
	if stored.Status != run.StatusQueued {
		t.Fatalf("stored status = %s, want %s", stored.Status, run.StatusQueued)
	}
	if stored.Task.Workspace.EffectiveType() != run.WorkspaceSourceNone {
		t.Fatalf("stored workspace = %q, want none", stored.Task.Workspace.EffectiveType())
	}

	queuedID, err := queue.Dequeue(ctx)
	if err != nil {
		t.Fatalf("dequeue run: %v", err)
	}
	if queuedID.String() != created.ID {
		t.Fatalf("queued ID = %s, want %s", queuedID, created.ID)
	}

	assertEvent(t, publisher.events, outbound.EventRunCreated, run.RunID(created.ID))
}

func TestCreateRunStoresConfiguredWorkspace(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	queue := &fakeRunQueue{}
	publisher := &recordingPublisher{}
	now := time.Unix(100, 0).UTC()

	handler := NewCreateRunHandler(
		repo,
		queue,
		publisher,
		fixedIDGenerator{id: "run_test"},
		WithCreateRunClock(func() time.Time { return now }),
	)

	created, err := handler.CreateRun(ctx, inbound.CreateRunCommand{
		Prompt:    "do work",
		Workspace: inbound.WorkspaceSourceInput{Type: string(run.WorkspaceSourceConfigured)},
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	stored, err := repo.FindByID(ctx, run.RunID(created.ID))
	if err != nil {
		t.Fatalf("find stored run: %v", err)
	}
	if stored.Task.Workspace.Type != run.WorkspaceSourceConfigured {
		t.Fatalf("stored workspace = %q, want configured", stored.Task.Workspace.Type)
	}
	if created.Task.Workspace.Type != string(run.WorkspaceSourceConfigured) {
		t.Fatalf("view workspace = %q, want configured", created.Task.Workspace.Type)
	}
}

func TestCreateRunRejectsUnknownWorkspace(t *testing.T) {
	ctx := context.Background()
	handler := NewCreateRunHandler(
		newFakeRunRepository(),
		&fakeRunQueue{},
		&recordingPublisher{},
		fixedIDGenerator{id: "run_test"},
	)

	_, err := handler.CreateRun(ctx, inbound.CreateRunCommand{
		Prompt:    "do work",
		Workspace: inbound.WorkspaceSourceInput{Type: "snapshot"},
	})
	if !errors.Is(err, inbound.ErrInvalidInput) {
		t.Fatalf("CreateRun() error = %v, want invalid input", err)
	}
}

type fixedIDGenerator struct {
	id run.RunID
}

func (g fixedIDGenerator) NewRunID() (run.RunID, error) {
	return g.id, nil
}

type fakeRunRepository struct {
	items              map[run.RunID]*run.Run
	beforeSaveIfStatus func()
}

func newFakeRunRepository() *fakeRunRepository {
	return &fakeRunRepository{
		items: make(map[run.RunID]*run.Run),
	}
}

func (r *fakeRunRepository) Save(_ context.Context, item *run.Run) error {
	r.items[item.ID] = item.Clone()

	return nil
}

func (r *fakeRunRepository) SaveIfStatus(_ context.Context, item *run.Run, expected run.Status) (bool, error) {
	if r.beforeSaveIfStatus != nil {
		r.beforeSaveIfStatus()
	}

	stored, ok := r.items[item.ID]
	if !ok {
		return false, run.ErrRunNotFound
	}
	if stored.Status != expected {
		return false, nil
	}

	r.items[item.ID] = item.Clone()

	return true, nil
}

func (r *fakeRunRepository) FindByID(_ context.Context, id run.RunID) (*run.Run, error) {
	item, ok := r.items[id]
	if !ok {
		return nil, run.ErrRunNotFound
	}

	return item.Clone(), nil
}

type fakeRunQueue struct {
	ids []run.RunID
}

func (q *fakeRunQueue) Enqueue(_ context.Context, id run.RunID) error {
	q.ids = append(q.ids, id)

	return nil
}

func (q *fakeRunQueue) Dequeue(context.Context) (run.RunID, error) {
	if len(q.ids) == 0 {
		return "", outbound.ErrRunQueueEmpty
	}

	id := q.ids[0]
	q.ids = q.ids[1:]

	return id, nil
}

type recordingPublisher struct {
	events []outbound.Event
}

func (p *recordingPublisher) Publish(_ context.Context, event outbound.Event) error {
	p.events = append(p.events, event)

	return nil
}

func assertEvent(t *testing.T, events []outbound.Event, eventType string, id run.RunID) {
	t.Helper()

	for _, event := range events {
		if event.Type == eventType && event.RunID == id {
			return
		}
	}

	t.Fatalf("event %s for run %s not found", eventType, id)
}
