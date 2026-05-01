package memory

import (
	"context"
	"errors"
	"sort"
	"sync"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

var errNilRun = errors.New("run cannot be nil")

// RunRepository stores runs in process memory.
type RunRepository struct {
	mu   sync.RWMutex
	runs map[run.RunID]*run.Run
}

var _ outbound.RunRepository = (*RunRepository)(nil)

// NewRunRepository creates an empty in-memory run repository.
func NewRunRepository() *RunRepository {
	return &RunRepository{
		runs: make(map[run.RunID]*run.Run),
	}
}

// Save stores a detached copy of the run.
func (r *RunRepository) Save(_ context.Context, item *run.Run) error {
	if item == nil {
		return errNilRun
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.runs[item.ID] = item.Clone()

	return nil
}

// SaveIfStatus stores a detached copy only when the stored status matches.
func (r *RunRepository) SaveIfStatus(_ context.Context, item *run.Run, expected run.Status) (bool, error) {
	if item == nil {
		return false, errNilRun
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	stored, ok := r.runs[item.ID]
	if !ok {
		return false, outbound.ErrRunNotFound
	}
	if stored.Status != expected {
		return false, nil
	}

	r.runs[item.ID] = item.Clone()

	return true, nil
}

// FindByID returns a detached copy of a run by ID.
func (r *RunRepository) FindByID(_ context.Context, id run.RunID) (*run.Run, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	item, ok := r.runs[id]
	if !ok {
		return nil, outbound.ErrRunNotFound
	}

	return item.Clone(), nil
}

// List returns detached copies of all runs ordered by creation time and ID.
func (r *RunRepository) List(context.Context) ([]*run.Run, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]*run.Run, 0, len(r.runs))
	for _, item := range r.runs {
		items = append(items, item.Clone())
	}

	sort.Slice(items, func(i int, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID < items[j].ID
		}

		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})

	return items, nil
}
