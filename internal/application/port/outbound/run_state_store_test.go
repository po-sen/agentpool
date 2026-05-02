package outbound_test

import (
	"context"
	"testing"
	"time"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestRunStateStoreContract(t *testing.T) {
	var store outbound.RunStateStore = fakeRunStateStore{}

	item, err := run.New("run_test", run.TaskSpec{Prompt: "do work"}, time.Unix(100, 0).UTC())
	if err != nil {
		t.Fatalf("new run: %v", err)
	}

	saved, err := store.SaveIfStatus(context.Background(), item, run.StatusQueued)
	if err != nil {
		t.Fatalf("SaveIfStatus() error = %v", err)
	}
	if !saved {
		t.Fatal("SaveIfStatus() saved = false, want true")
	}
}

type fakeRunStateStore struct{}

func (fakeRunStateStore) SaveIfStatus(context.Context, *run.Run, run.Status) (bool, error) {
	return true, nil
}
