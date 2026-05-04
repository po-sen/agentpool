package run

import (
	"errors"
	"testing"
)

func TestRunSentinelErrors(t *testing.T) {
	tests := []error{
		ErrEmptyRunID,
		ErrEmptyPrompt,
		ErrPromptTooLong,
		ErrTooManyAttachments,
		ErrMissingAttachmentFilename,
		ErrUnsafeAttachmentFilename,
		ErrAttachmentTooLarge,
		ErrAttachmentsTooLarge,
		ErrUnsupportedAttachmentType,
		ErrAttachmentNotText,
		ErrAttachmentSizeMismatch,
		ErrInvalidTransition,
		ErrUnknownStatus,
		ErrRunNotFound,
		ErrStepNotFound,
		ErrStepAlreadyEnded,
		ErrInvalidStepName,
	}

	for _, err := range tests {
		if !errors.Is(err, err) {
			t.Fatalf("sentinel error %v should match itself", err)
		}
	}
}
