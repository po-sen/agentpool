package run

import "context"

// Repository is the collection-like interface for the Run aggregate.
type Repository interface {
	Save(context.Context, *Run) error
	FindByID(context.Context, RunID) (*Run, error)
}
