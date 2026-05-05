package outbound

import (
	"testing"

	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestIDGeneratorContract(t *testing.T) {
	var ids IDGenerator = contractIDGenerator{}

	id, err := ids.NewRunID()
	if err != nil {
		t.Fatalf("NewRunID() error = %v", err)
	}
	if id != "run_test" {
		t.Fatalf("id = %q, want run_test", id)
	}
}

type contractIDGenerator struct{}

func (contractIDGenerator) NewRunID() (run.RunID, error) {
	return "run_test", nil
}
