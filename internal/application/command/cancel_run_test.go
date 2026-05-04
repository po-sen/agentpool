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

func TestCancelRunMarksRunCancelled(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	publisher := &recordingPublisher{}
	now := time.Unix(100, 0).UTC()

	item, err := run.New("run_test", run.TaskSpec{Prompt: "do work"}, now)
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	if err := repo.Save(ctx, item); err != nil {
		t.Fatalf("save run: %v", err)
	}

	handler := NewCancelRunHandler(
		repo,
		repo,
		publisher,
		WithCancelRunClock(func() time.Time { return now.Add(time.Second) }),
	)

	cancelled, err := handler.CancelRun(ctx, inbound.CancelRunCommand{RunID: item.ID.String()})
	if err != nil {
		t.Fatalf("cancel run: %v", err)
	}
	if cancelled.Status != string(run.StatusCancelled) {
		t.Fatalf("cancelled status = %s, want %s", cancelled.Status, run.StatusCancelled)
	}

	stored, err := repo.FindByID(ctx, item.ID)
	if err != nil {
		t.Fatalf("find stored run: %v", err)
	}
	if stored.Status != run.StatusCancelled {
		t.Fatalf("stored status = %s, want %s", stored.Status, run.StatusCancelled)
	}

	assertEvent(t, publisher.events, outbound.EventRunCancelled, item.ID)
}

func TestCancelRunReturnsConflictWhenStoredStatusChanges(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	publisher := &recordingPublisher{}
	now := time.Unix(100, 0).UTC()

	item, err := run.New("run_test", run.TaskSpec{Prompt: "do work"}, now)
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	if err := repo.Save(ctx, item); err != nil {
		t.Fatalf("save run: %v", err)
	}
	repo.beforeSaveIfStatus = func() {
		stored := repo.items[item.ID].Clone()
		stored.Status = run.StatusCompleted
		repo.items[item.ID] = stored
	}

	handler := NewCancelRunHandler(
		repo,
		repo,
		publisher,
		WithCancelRunClock(func() time.Time { return now.Add(time.Second) }),
	)

	_, err = handler.CancelRun(ctx, inbound.CancelRunCommand{RunID: item.ID.String()})
	if !errors.Is(err, inbound.ErrConflict) {
		t.Fatalf("cancel run error = %v, want %v", err, inbound.ErrConflict)
	}

	stored, err := repo.FindByID(ctx, item.ID)
	if err != nil {
		t.Fatalf("find stored run: %v", err)
	}
	if stored.Status != run.StatusCompleted {
		t.Fatalf("stored status = %s, want %s", stored.Status, run.StatusCompleted)
	}
	if len(publisher.events) != 0 {
		t.Fatalf("published events = %d, want 0", len(publisher.events))
	}
}
