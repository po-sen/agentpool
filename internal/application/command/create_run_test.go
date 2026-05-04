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

	queuedID, err := queue.Dequeue(ctx)
	if err != nil {
		t.Fatalf("dequeue run: %v", err)
	}
	if queuedID.String() != created.ID {
		t.Fatalf("queued ID = %s, want %s", queuedID, created.ID)
	}

	assertEvent(t, publisher.events, outbound.EventRunCreated, run.RunID(created.ID))
}

func TestCreateRunStoresAttachments(t *testing.T) {
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
	content := []byte("# Demo\n")

	created, err := handler.CreateRun(ctx, inbound.CreateRunCommand{
		Prompt: "do work",
		Attachments: []inbound.AttachmentInput{
			{
				Filename:  "README.md",
				MediaType: "text/markdown",
				Content:   content,
				SizeBytes: int64(len(content)),
			},
		},
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	content[0] = 'X'

	stored, err := repo.FindByID(ctx, run.RunID(created.ID))
	if err != nil {
		t.Fatalf("find stored run: %v", err)
	}
	if len(stored.Task.Attachments) != 1 {
		t.Fatalf("len(stored attachments) = %d, want 1", len(stored.Task.Attachments))
	}
	if string(stored.Task.Attachments[0].Content) != "# Demo\n" {
		t.Fatalf("stored attachment content = %q, want original", stored.Task.Attachments[0].Content)
	}
	if len(created.Task.Attachments) != 1 {
		t.Fatalf("len(view attachments) = %d, want 1", len(created.Task.Attachments))
	}
	if created.Task.Attachments[0].Filename != "README.md" {
		t.Fatalf("view attachment filename = %q, want README.md", created.Task.Attachments[0].Filename)
	}
}

func TestCreateRunRejectsEmptyPrompt(t *testing.T) {
	ctx := context.Background()
	handler := NewCreateRunHandler(
		newFakeRunRepository(),
		&fakeRunQueue{},
		&recordingPublisher{},
		fixedIDGenerator{id: "run_test"},
	)

	_, err := handler.CreateRun(ctx, inbound.CreateRunCommand{Prompt: " "})
	if !errors.Is(err, inbound.ErrInvalidInput) {
		t.Fatalf("CreateRun() error = %v, want invalid input", err)
	}
}

func TestCreateRunRejectsInvalidAttachment(t *testing.T) {
	ctx := context.Background()
	handler := NewCreateRunHandler(
		newFakeRunRepository(),
		&fakeRunQueue{},
		&recordingPublisher{},
		fixedIDGenerator{id: "run_test"},
	)

	_, err := handler.CreateRun(ctx, inbound.CreateRunCommand{
		Prompt: "do work",
		Attachments: []inbound.AttachmentInput{
			{
				Filename:  "../README.md",
				MediaType: "text/markdown",
				Content:   []byte("bad"),
				SizeBytes: 3,
			},
		},
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
