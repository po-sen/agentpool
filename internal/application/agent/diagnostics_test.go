package agent

import "testing"

func TestGeneratedToolCallIDNormalizesToolNames(t *testing.T) {
	got := generatedToolCallID("Sandbox Exec!", 2)
	if got != "sandbox_exec_3" {
		t.Fatalf("generatedToolCallID() = %q, want sandbox_exec_3", got)
	}
}
