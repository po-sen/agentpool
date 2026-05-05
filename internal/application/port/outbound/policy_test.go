package outbound

import (
	"context"
	"testing"
)

func TestPolicyDecisionPortContract(t *testing.T) {
	var policy PolicyDecisionPort = contractPolicyDecision{}

	decision, err := policy.Decide(context.Background(), PolicyDecisionRequest{RunID: "run_test"})
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if !decision.Allowed {
		t.Fatal("Allowed = false, want true")
	}
}

type contractPolicyDecision struct{}

func (contractPolicyDecision) Decide(context.Context, PolicyDecisionRequest) (PolicyDecision, error) {
	return PolicyDecision{Allowed: true}, nil
}
