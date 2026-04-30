package outbound

import (
	"context"

	"github.com/po-sen/agentpool/internal/domain/run"
)

// RunRepository stores and retrieves runs.
type RunRepository interface {
	Save(context.Context, *run.Run) error
	FindByID(context.Context, run.RunID) (*run.Run, error)
	List(context.Context) ([]*run.Run, error)
}
