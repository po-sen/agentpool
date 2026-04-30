package outbound

import (
	"context"

	"github.com/po-sen/agentpool/internal/domain/run"
)

// RunQueue dispatches runs to workers.
type RunQueue interface {
	Enqueue(context.Context, run.RunID) error
	Dequeue(context.Context) (run.RunID, error)
}
