package policy

import (
	"context"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

// AllowAllDecision allows every policy request.
type AllowAllDecision struct{}

var _ outbound.PolicyDecisionPort = (*AllowAllDecision)(nil)

// NewAllowAllDecision creates an allow-all policy adapter.
func NewAllowAllDecision() *AllowAllDecision {
	return &AllowAllDecision{}
}

// Decide allows every request.
func (d *AllowAllDecision) Decide(context.Context, outbound.PolicyDecisionRequest) (outbound.PolicyDecision, error) {
	return outbound.PolicyDecision{Allowed: true, Reason: "allow-all"}, nil
}
