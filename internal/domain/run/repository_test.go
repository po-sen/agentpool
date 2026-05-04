package run

import (
	"context"
	"testing"
	"time"
)

func TestRepositoryContract(t *testing.T) {
	var repo Repository = fakeRepository{}

	item, err := New("run_test", TaskSpec{Prompt: "do work"}, time.Unix(100, 0).UTC())
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

func (fakeRepository) Save(context.Context, *Run) error {
	return nil
}

func (fakeRepository) FindByID(context.Context, RunID) (*Run, error) {
	return New("run_test", TaskSpec{Prompt: "do work"}, time.Unix(100, 0).UTC())
}
