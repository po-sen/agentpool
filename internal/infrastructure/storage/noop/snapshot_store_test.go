package noop

import (
	"context"
	"errors"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestSnapshotStoreImplementsPort(_ *testing.T) {
	var _ outbound.SnapshotStore = (*SnapshotStore)(nil)
}

func TestSnapshotStoreReturnsNotFound(t *testing.T) {
	store := NewSnapshotStore()

	_, err := store.OpenSnapshot(context.Background(), outbound.SnapshotOpenRequest{SnapshotID: "wsnap_test"})
	if !errors.Is(err, outbound.ErrSnapshotNotFound) {
		t.Fatalf("OpenSnapshot() error = %v, want %v", err, outbound.ErrSnapshotNotFound)
	}
}
