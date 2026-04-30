package inbound_test

import (
	"errors"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/inbound"
)

func TestNewInvalidInputErrorWrapsCauseAndMatchesKind(t *testing.T) {
	cause := errors.New("prompt is required")

	err := inbound.NewInvalidInputError(cause)
	if !errors.Is(err, inbound.ErrInvalidInput) {
		t.Fatalf("error does not match invalid input kind: %v", err)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("error does not wrap cause: %v", err)
	}
}

func TestNewConflictErrorWithNilCauseReturnsKind(t *testing.T) {
	err := inbound.NewConflictError(nil)
	if !errors.Is(err, inbound.ErrConflict) {
		t.Fatalf("error = %v, want conflict kind", err)
	}
}
