package file

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestSnapshotStoreImplementsPort(_ *testing.T) {
	var _ outbound.SnapshotStore = (*SnapshotStore)(nil)
}

func TestNewSnapshotStoreRejectsEmptyDirectory(t *testing.T) {
	_, err := NewSnapshotStore(" ")
	if err == nil {
		t.Fatal("NewSnapshotStore() error = nil, want error")
	}
}

func TestOpenSnapshotOpensExistingSnapshot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "wsnap_demo.zip"), []byte("zip content"), 0o600); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	store := newTestStore(t, root)

	reader, err := store.OpenSnapshot(context.Background(), outbound.SnapshotOpenRequest{SnapshotID: "wsnap_demo"})
	if err != nil {
		t.Fatalf("OpenSnapshot() error = %v", err)
	}
	defer func() {
		_ = reader.Close()
	}()

	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if string(content) != "zip content" {
		t.Fatalf("snapshot content = %q, want zip content", content)
	}
}

func TestOpenSnapshotReturnsNotFoundForMissingSnapshot(t *testing.T) {
	store := newTestStore(t, t.TempDir())

	_, err := store.OpenSnapshot(context.Background(), outbound.SnapshotOpenRequest{SnapshotID: "missing"})
	if !errors.Is(err, outbound.ErrSnapshotNotFound) {
		t.Fatalf("OpenSnapshot() error = %v, want %v", err, outbound.ErrSnapshotNotFound)
	}
}

func TestOpenSnapshotRejectsUnsafeIDs(t *testing.T) {
	root := t.TempDir()
	store := newTestStore(t, root)
	tests := []string{
		"",
		" ",
		"../secret",
		"..",
		"safe/secret",
		`safe\secret`,
		"/absolute",
		"safe..secret",
		"safe secret",
		"safe:secret",
	}

	for _, snapshotID := range tests {
		t.Run(strings.ReplaceAll(snapshotID, "/", "_"), func(t *testing.T) {
			reader, err := store.OpenSnapshot(context.Background(), outbound.SnapshotOpenRequest{SnapshotID: snapshotID})
			if reader != nil {
				_ = reader.Close()
				t.Fatal("OpenSnapshot() reader != nil, want nil")
			}
			if !errors.Is(err, outbound.ErrInvalidSnapshotID) {
				t.Fatalf("OpenSnapshot() error = %v, want %v", err, outbound.ErrInvalidSnapshotID)
			}
		})
	}
}

func newTestStore(t *testing.T, root string) *SnapshotStore {
	t.Helper()

	store, err := NewSnapshotStore(root)
	if err != nil {
		t.Fatalf("NewSnapshotStore() error = %v", err)
	}

	return store
}
