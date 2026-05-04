package diff

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

// Collector compares a materialized workspace against its baseline.
type Collector struct{}

var _ outbound.WorkspaceChangeCollector = (*Collector)(nil)

type fileFingerprint struct {
	size   int64
	digest [sha256.Size]byte
}

// NewCollector creates a workspace diff collector.
func NewCollector() *Collector {
	return &Collector{}
}

// CollectWorkspaceChanges returns file-level workspace changes.
func (c *Collector) CollectWorkspaceChanges(
	ctx context.Context,
	workspace outbound.Workspace,
) ([]outbound.WorkspaceChange, error) {
	if workspace.Path == "" || workspace.BasePath == "" {
		return nil, nil
	}

	baseFiles, err := collectFiles(ctx, workspace.BasePath)
	if err != nil {
		return nil, err
	}
	currentFiles, err := collectFiles(ctx, workspace.Path)
	if err != nil {
		return nil, err
	}

	changes := compareFiles(baseFiles, currentFiles)
	sort.Slice(changes, func(i int, j int) bool {
		if changes[i].Path == changes[j].Path {
			return changes[i].Status < changes[j].Status
		}

		return changes[i].Path < changes[j].Path
	})

	return changes, nil
}

func collectFiles(ctx context.Context, root string) (map[string]fileFingerprint, error) {
	files := make(map[string]fileFingerprint)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		return collectFileEntry(ctx, root, files, path, entry, err)
	})
	if err != nil {
		return nil, fmt.Errorf("collect workspace files: %w", err)
	}

	return files, nil
}

func collectFileEntry(
	ctx context.Context,
	root string,
	files map[string]fileFingerprint,
	path string,
	entry os.DirEntry,
	walkErr error,
) error {
	if walkErr != nil {
		return walkErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if path == root || entry.IsDir() {
		return nil
	}

	rel, err := safeRel(root, path)
	if err != nil {
		return err
	}
	info, err := entry.Info()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("workspace change collection only supports regular files: %s", rel)
	}

	fingerprint, err := fingerprintFile(path, info.Size())
	if err != nil {
		return err
	}
	files[rel] = fingerprint

	return nil
}

func compareFiles(
	baseFiles map[string]fileFingerprint,
	currentFiles map[string]fileFingerprint,
) []outbound.WorkspaceChange {
	var changes []outbound.WorkspaceChange
	for path, current := range currentFiles {
		base, ok := baseFiles[path]
		if !ok {
			changes = append(changes, outbound.WorkspaceChange{Path: path, Status: outbound.WorkspaceChangeAdded})
			continue
		}
		if base != current {
			changes = append(changes, outbound.WorkspaceChange{Path: path, Status: outbound.WorkspaceChangeModified})
		}
	}
	for path := range baseFiles {
		if _, ok := currentFiles[path]; ok {
			continue
		}
		changes = append(changes, outbound.WorkspaceChange{Path: path, Status: outbound.WorkspaceChangeDeleted})
	}

	return changes
}

func fingerprintFile(path string, size int64) (fileFingerprint, error) {
	file, err := os.Open(path) // #nosec G304 -- path comes from WalkDir under a resolved workspace root.
	if err != nil {
		return fileFingerprint{}, err
	}
	defer func() {
		_ = file.Close()
	}()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fileFingerprint{}, err
	}
	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))

	return fileFingerprint{size: size, digest: digest}, nil
}

func safeRel(root string, path string) (string, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", fmt.Errorf("workspace path escapes root: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("workspace path escapes root: %s", path)
	}

	return filepath.ToSlash(rel), nil
}
