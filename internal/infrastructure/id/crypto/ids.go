package crypto

import (
	"crypto/rand"
	"encoding/hex"
	"io"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

// Generator creates opaque run IDs using crypto/rand.
type Generator struct{}

var _ outbound.IDGenerator = (*Generator)(nil)

// NewGenerator creates a run ID generator.
func NewGenerator() *Generator {
	return &Generator{}
}

// NewRunID creates a new run ID.
func (g *Generator) NewRunID() (run.RunID, error) {
	bytes := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return "", err
	}

	return run.RunID("run_" + hex.EncodeToString(bytes)), nil
}
