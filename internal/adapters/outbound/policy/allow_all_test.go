package policy_test

import (
	"context"
	"testing"

	"github.com/po-sen/agentpool/internal/adapters/outbound/policy"
	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestAllowAllDecisionAllowsRequest(t *testing.T) {
	decision, err := policy.NewAllowAllDecision().Decide(context.Background(), outbound.PolicyDecisionRequest{
		RunID: "run_test",
	})
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if !decision.Allowed {
		t.Fatal("decision.Allowed = false, want true")
	}
	if decision.Reason == "" {
		t.Fatal("decision.Reason is empty")
	}
}
