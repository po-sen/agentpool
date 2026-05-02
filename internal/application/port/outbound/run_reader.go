package outbound

import (
	"context"

	"github.com/po-sen/agentpool/internal/domain/run"
)

// RunReader provides read/query access for runs.
type RunReader interface {
	List(context.Context) ([]*run.Run, error)
}
