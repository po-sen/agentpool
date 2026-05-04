package outbound

import (
	"context"
	"testing"
	"time"

	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestRunReaderContract(t *testing.T) {
	var reader RunReader = fakeRunReader{}

	items, err := reader.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
}

type fakeRunReader struct{}

func (fakeRunReader) List(context.Context) ([]*run.Run, error) {
	item, err := run.New("run_test", run.TaskSpec{Prompt: "do work"}, time.Unix(100, 0).UTC())
	if err != nil {
		return nil, err
	}

	return []*run.Run{item}, nil
}
