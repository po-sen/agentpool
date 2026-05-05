package outbound

import (
	"context"

	"github.com/po-sen/agentpool/internal/domain/run"
)

// PolicyDecisionRequest describes a policy evaluation request.
type PolicyDecisionRequest struct {
	RunID run.RunID
	Task  run.TaskSpec
}

// PolicyDecision describes whether a requested action is allowed.
type PolicyDecision struct {
	Allowed bool
	Reason  string
}

// PolicyDecisionPort evaluates policy decisions.
type PolicyDecisionPort interface {
	Decide(context.Context, PolicyDecisionRequest) (PolicyDecision, error)
}
