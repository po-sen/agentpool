package outbound

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestSnapshotStoreContract(t *testing.T) {
	var store SnapshotStore = contractSnapshotStore{}

	body, err := store.OpenSnapshot(context.Background(), SnapshotOpenRequest{SnapshotID: "wsnap_test"})
	if err != nil {
		t.Fatalf("OpenSnapshot() error = %v", err)
	}
	defer func() {
		_ = body.Close()
	}()

	content, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(content) != "snapshot" {
		t.Fatalf("content = %q, want snapshot", content)
	}
}

type contractSnapshotStore struct{}

func (contractSnapshotStore) OpenSnapshot(context.Context, SnapshotOpenRequest) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("snapshot")), nil
}
