package outbound

import "github.com/po-sen/agentpool/internal/domain/run"

// IDGenerator creates run IDs.
type IDGenerator interface {
	NewRunID() (run.RunID, error)
}
