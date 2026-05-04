package run

import "errors"

// ErrEmptyRunID is returned when constructing a run without an ID.
var ErrEmptyRunID = errors.New("run id is required")

// ErrEmptyPrompt is returned when constructing a run without a task prompt.
var ErrEmptyPrompt = errors.New("task prompt is required")

// ErrPromptTooLong is returned when the requested prompt exceeds the limit.
var ErrPromptTooLong = errors.New("task prompt is too long")

// ErrTooManyAttachments is returned when a task includes too many files.
var ErrTooManyAttachments = errors.New("too many task attachments")

// ErrMissingAttachmentFilename is returned when an attachment filename is empty.
var ErrMissingAttachmentFilename = errors.New("attachment filename is required")

// ErrUnsafeAttachmentFilename is returned when an attachment filename is not a safe relative path.
var ErrUnsafeAttachmentFilename = errors.New("attachment filename is unsafe")

// ErrAttachmentTooLarge is returned when one attachment exceeds the per-file size limit.
var ErrAttachmentTooLarge = errors.New("attachment is too large")

// ErrAttachmentsTooLarge is returned when all attachments exceed the total size limit.
var ErrAttachmentsTooLarge = errors.New("attachments are too large")

// ErrUnsupportedAttachmentType is returned when an attachment extension is not supported.
var ErrUnsupportedAttachmentType = errors.New("attachment type is not supported")

// ErrAttachmentNotText is returned when an attachment contains non-UTF-8 content.
var ErrAttachmentNotText = errors.New("attachment must be UTF-8 text")

// ErrAttachmentSizeMismatch is returned when attachment metadata does not match content length.
var ErrAttachmentSizeMismatch = errors.New("attachment size does not match content")

// ErrInvalidTransition is returned when a lifecycle transition is not allowed.
var ErrInvalidTransition = errors.New("invalid run status transition")

// ErrUnknownStatus is returned when a transition targets an unsupported status.
var ErrUnknownStatus = errors.New("unknown run status")

// ErrRunNotFound is returned when a run ID does not exist.
var ErrRunNotFound = errors.New("run not found")

// ErrStepNotFound is returned when a run step cannot be found.
var ErrStepNotFound = errors.New("run step not found")

// ErrStepAlreadyEnded is returned when a run step has already reached a terminal state.
var ErrStepAlreadyEnded = errors.New("run step already ended")

// ErrInvalidStepName is returned when a run step name is empty.
var ErrInvalidStepName = errors.New("run step name is required")
