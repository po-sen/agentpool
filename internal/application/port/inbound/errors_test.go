package inbound

import (
	"errors"
	"testing"
)

func TestNewInvalidInputErrorWrapsCauseAndMatchesKind(t *testing.T) {
	cause := errors.New("prompt is required")

	err := NewInvalidInputError(cause)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("error does not match invalid input kind: %v", err)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("error does not wrap cause: %v", err)
	}
}

func TestNewConflictErrorWithNilCauseReturnsKind(t *testing.T) {
	err := NewConflictError(nil)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("error = %v, want conflict kind", err)
	}
}
