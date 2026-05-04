package outbound

import (
	"context"
	"io"
)

// SnapshotOpenRequest identifies a stored workspace snapshot.
type SnapshotOpenRequest struct {
	SnapshotID string
}

// SnapshotStore opens stored workspace snapshot archives.
type SnapshotStore interface {
	OpenSnapshot(context.Context, SnapshotOpenRequest) (io.ReadCloser, error)
}
