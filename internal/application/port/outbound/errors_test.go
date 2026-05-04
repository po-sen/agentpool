package outbound

import (
	"errors"
	"testing"
)

func TestOutboundSentinelErrors(t *testing.T) {
	if !errors.Is(ErrRunQueueEmpty, ErrRunQueueEmpty) {
		t.Fatal("ErrRunQueueEmpty should match itself")
	}
	if !errors.Is(ErrSnapshotNotFound, ErrSnapshotNotFound) {
		t.Fatal("ErrSnapshotNotFound should match itself")
	}
}
