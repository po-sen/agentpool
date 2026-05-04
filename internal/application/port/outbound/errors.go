package outbound

import "errors"

var (
	// ErrRunQueueEmpty is returned when no queued runs are available.
	ErrRunQueueEmpty = errors.New("run queue is empty")
	// ErrSnapshotNotFound is returned when a workspace snapshot cannot be found.
	ErrSnapshotNotFound = errors.New("workspace snapshot not found")
	// ErrInvalidSnapshotID is returned when a workspace snapshot ID is invalid.
	ErrInvalidSnapshotID = errors.New("invalid workspace snapshot id")
)
