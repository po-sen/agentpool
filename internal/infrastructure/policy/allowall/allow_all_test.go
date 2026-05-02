package allowall_test

import (
	"context"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/infrastructure/policy/allowall"
)

func TestDecisionAllowsRequest(t *testing.T) {
	decision, err := allowall.NewDecision().Decide(context.Background(), outbound.PolicyDecisionRequest{
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
