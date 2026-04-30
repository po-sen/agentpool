package approval_test

import (
	"testing"

	"github.com/po-sen/agentpool/internal/domain/approval"
)

func TestDecisionValues(t *testing.T) {
	tests := []struct {
		name string
		got  approval.Decision
		want string
	}{
		{name: "approved", got: approval.DecisionApproved, want: "approved"},
		{name: "rejected", got: approval.DecisionRejected, want: "rejected"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.got) != tt.want {
				t.Fatalf("decision = %q, want %q", tt.got, tt.want)
			}
		})
	}
}
