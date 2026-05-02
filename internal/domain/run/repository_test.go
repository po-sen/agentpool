package run_test

import (
	"context"
	"testing"
	"time"

	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestRepositoryContract(t *testing.T) {
	var repo run.Repository = fakeRepository{}

	item, err := run.New("run_test", run.TaskSpec{Prompt: "do work"}, time.Unix(100, 0).UTC())
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	if err := repo.Save(context.Background(), item); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if _, err := repo.FindByID(context.Background(), item.ID); err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
}

type fakeRepository struct{}

func (fakeRepository) Save(context.Context, *run.Run) error {
	return nil
}

func (fakeRepository) FindByID(context.Context, run.RunID) (*run.Run, error) {
	return run.New("run_test", run.TaskSpec{Prompt: "do work"}, time.Unix(100, 0).UTC())
}
