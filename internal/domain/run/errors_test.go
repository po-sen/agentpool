package run_test

import (
	"errors"
	"testing"

	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestRunSentinelErrors(t *testing.T) {
	tests := []error{
		run.ErrEmptyRunID,
		run.ErrEmptyPrompt,
		run.ErrPromptTooLong,
		run.ErrInvalidTransition,
		run.ErrUnknownStatus,
		run.ErrRunNotFound,
	}

	for _, err := range tests {
		if !errors.Is(err, err) {
			t.Fatalf("sentinel error %v should match itself", err)
		}
	}
}
