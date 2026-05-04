package file

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

// SnapshotStore opens snapshot zip files from a configured directory.
type SnapshotStore struct {
	snapshotDir string
}

var _ outbound.SnapshotStore = (*SnapshotStore)(nil)

// NewSnapshotStore creates a file-backed snapshot store rooted at snapshotDir.
func NewSnapshotStore(snapshotDir string) (*SnapshotStore, error) {
	cleanDir := strings.TrimSpace(snapshotDir)
	if cleanDir == "" {
		return nil, errors.New("snapshot directory is required")
	}

	return &SnapshotStore{snapshotDir: filepath.Clean(cleanDir)}, nil
}

// OpenSnapshot opens <snapshot_dir>/<snapshot_id>.zip.
func (s *SnapshotStore) OpenSnapshot(ctx context.Context, request outbound.SnapshotOpenRequest) (io.ReadCloser, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	snapshotID, err := validateSnapshotID(request.SnapshotID)
	if err != nil {
		return nil, err
	}

	snapshotPath, err := s.snapshotPath(snapshotID)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(snapshotPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, outbound.ErrSnapshotNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("stat snapshot: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("snapshot file is not regular: %s", snapshotID)
	}

	file, err := os.Open(snapshotPath) // #nosec G304 -- snapshot ID validation confines paths to snapshotDir.
	if errors.Is(err, os.ErrNotExist) {
		return nil, outbound.ErrSnapshotNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("open snapshot: %w", err)
	}

	return file, nil
}

func (s *SnapshotStore) snapshotPath(snapshotID string) (string, error) {
	snapshotPath := filepath.Join(s.snapshotDir, snapshotID+".zip")
	rel, err := filepath.Rel(s.snapshotDir, snapshotPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", invalidSnapshotID(snapshotID)
	}

	return snapshotPath, nil
}

func validateSnapshotID(rawSnapshotID string) (string, error) {
	snapshotID := strings.TrimSpace(rawSnapshotID)
	if snapshotID == "" {
		return "", invalidSnapshotID(rawSnapshotID)
	}
	if strings.Contains(snapshotID, "..") ||
		strings.ContainsAny(snapshotID, `/\`) ||
		filepath.IsAbs(snapshotID) {
		return "", invalidSnapshotID(rawSnapshotID)
	}
	for _, char := range snapshotID {
		if !isSafeSnapshotIDChar(char) {
			return "", invalidSnapshotID(rawSnapshotID)
		}
	}

	return snapshotID, nil
}

func isSafeSnapshotIDChar(char rune) bool {
	return (char >= 'a' && char <= 'z') ||
		(char >= 'A' && char <= 'Z') ||
		(char >= '0' && char <= '9') ||
		char == '_' ||
		char == '-' ||
		char == '.'
}

func invalidSnapshotID(snapshotID string) error {
	return fmt.Errorf("%w: %q", outbound.ErrInvalidSnapshotID, snapshotID)
}
