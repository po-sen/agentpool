package run

import "errors"

// ErrEmptyRunID is returned when constructing a run without an ID.
var ErrEmptyRunID = errors.New("run id is required")

// ErrEmptyPrompt is returned when constructing a run without a task prompt.
var ErrEmptyPrompt = errors.New("task prompt is required")

// ErrPromptTooLong is returned when the requested prompt exceeds the limit.
var ErrPromptTooLong = errors.New("task prompt is too long")

// ErrInvalidTransition is returned when a lifecycle transition is not allowed.
var ErrInvalidTransition = errors.New("invalid run status transition")

// ErrUnknownStatus is returned when a transition targets an unsupported status.
var ErrUnknownStatus = errors.New("unknown run status")
