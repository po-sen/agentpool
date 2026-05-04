package approval

import (
	"testing"
)

func TestDecisionValues(t *testing.T) {
	tests := []struct {
		name string
		got  Decision
		want string
	}{
		{name: "approved", got: DecisionApproved, want: "approved"},
		{name: "rejected", got: DecisionRejected, want: "rejected"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.got) != tt.want {
				t.Fatalf("decision = %q, want %q", tt.got, tt.want)
			}
		})
	}
}
