package allowall

import (
	"context"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

// Decision allows every policy request.
type Decision struct{}

var _ outbound.PolicyDecisionPort = (*Decision)(nil)

// NewDecision creates an allow-all policy implementation.
func NewDecision() *Decision {
	return &Decision{}
}

// Decide allows every request.
func (d *Decision) Decide(context.Context, outbound.PolicyDecisionRequest) (outbound.PolicyDecision, error) {
	return outbound.PolicyDecision{Allowed: true, Reason: "allow-all"}, nil
}
