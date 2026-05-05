package agent

import (
	"errors"
	"testing"
)

func TestAgentErrorReturnsSafeMessageAndWrapsCause(t *testing.T) {
	cause := errors.New("provider returned api_key=secret")

	err := newAgentError(ErrorCodeModelGenerateFailed, "", cause)

	if err.Error() != messageModelGenerateFailed {
		t.Fatalf("Error() = %q, want %q", err.Error(), messageModelGenerateFailed)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("agent error does not wrap cause %v", cause)
	}
	if err.Code != ErrorCodeModelGenerateFailed {
		t.Fatalf("Code = %q, want %q", err.Code, ErrorCodeModelGenerateFailed)
	}
}

func TestAgentErrorDefaultsUnknownCodeAndMessage(t *testing.T) {
	err := newAgentError("", "", nil)

	if err.Code != ErrorCodeUnknown {
		t.Fatalf("Code = %q, want %q", err.Code, ErrorCodeUnknown)
	}
	if err.Error() != messageUnknownAgentError {
		t.Fatalf("Error() = %q, want %q", err.Error(), messageUnknownAgentError)
	}
}
