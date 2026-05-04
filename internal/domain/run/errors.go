package run

import "errors"

// ErrEmptyRunID is returned when constructing a run without an ID.
var ErrEmptyRunID = errors.New("run id is required")

// ErrEmptyPrompt is returned when constructing a run without a task prompt.
var ErrEmptyPrompt = errors.New("task prompt is required")

// ErrPromptTooLong is returned when the requested prompt exceeds the limit.
var ErrPromptTooLong = errors.New("task prompt is too long")

// ErrUnknownWorkspaceSource is returned when a task requests an unsupported workspace source.
var ErrUnknownWorkspaceSource = errors.New("unknown workspace source")

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
