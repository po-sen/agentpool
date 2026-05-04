package noop

import (
	"context"
	"io"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

// SnapshotStore reports that no snapshots are available.
type SnapshotStore struct{}

var _ outbound.SnapshotStore = (*SnapshotStore)(nil)

// NewSnapshotStore creates a no-op snapshot store.
func NewSnapshotStore() *SnapshotStore {
	return &SnapshotStore{}
}

// OpenSnapshot returns ErrSnapshotNotFound for every snapshot.
func (s *SnapshotStore) OpenSnapshot(context.Context, outbound.SnapshotOpenRequest) (io.ReadCloser, error) {
	return nil, outbound.ErrSnapshotNotFound
}
