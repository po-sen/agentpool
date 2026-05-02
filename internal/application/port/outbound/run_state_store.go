package outbound

import (
	"context"

	"github.com/po-sen/agentpool/internal/domain/run"
)

// RunStateStore persists run state changes with concurrency protection.
type RunStateStore interface {
	SaveIfStatus(context.Context, *run.Run, run.Status) (bool, error)
}
