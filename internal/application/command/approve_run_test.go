package command

import (
	"context"
	"errors"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/inbound"
)

func TestApproveRunReturnsNotImplemented(t *testing.T) {
	handler := NewApproveRunHandler()

	_, err := handler.ApproveRun(context.Background(), inbound.ApproveRunCommand{
		RunID:    "run_test",
		Decision: "approved",
	})
	if !errors.Is(err, inbound.ErrApprovalNotImplemented) {
		t.Fatalf("ApproveRun() error = %v, want %v", err, inbound.ErrApprovalNotImplemented)
	}
}
