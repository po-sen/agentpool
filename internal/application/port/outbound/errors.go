package outbound

import "errors"

var (
	// ErrRunNotFound is returned when a run ID does not exist.
	ErrRunNotFound = errors.New("run not found")
	// ErrRunQueueEmpty is returned when no queued runs are available.
	ErrRunQueueEmpty = errors.New("run queue is empty")
)
