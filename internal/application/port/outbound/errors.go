package outbound

import "errors"

var (
	// ErrRunQueueEmpty is returned when no queued runs are available.
	ErrRunQueueEmpty = errors.New("run queue is empty")
)
