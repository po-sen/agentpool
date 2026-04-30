package ids

import (
	"crypto/rand"
	"encoding/hex"
	"io"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

// CryptoGenerator creates opaque run IDs using crypto/rand.
type CryptoGenerator struct{}

var _ outbound.IDGenerator = (*CryptoGenerator)(nil)

// NewCryptoGenerator creates a run ID generator.
func NewCryptoGenerator() *CryptoGenerator {
	return &CryptoGenerator{}
}

// NewRunID creates a new run ID.
func (g *CryptoGenerator) NewRunID() (run.RunID, error) {
	bytes := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return "", err
	}

	return run.RunID("run_" + hex.EncodeToString(bytes)), nil
}
