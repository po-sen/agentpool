package command

import (
	"context"

	"github.com/po-sen/agentpool/internal/application/port/inbound"
)

var _ inbound.ApproveRunUseCase = (*ApproveRunHandler)(nil)

// ApproveRunHandler marks the future approval command boundary.
type ApproveRunHandler struct{}

// NewApproveRunHandler creates the approval placeholder command handler.
func NewApproveRunHandler() *ApproveRunHandler {
	return &ApproveRunHandler{}
}

// ApproveRun is intentionally a placeholder until approvals are modeled.
func (h *ApproveRunHandler) ApproveRun(context.Context, inbound.ApproveRunCommand) (inbound.RunView, error) {
	return inbound.RunView{}, inbound.ErrApprovalNotImplemented
}
