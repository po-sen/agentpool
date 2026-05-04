package snapshot

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

const (
	defaultMaxFiles      = 2000
	defaultMaxTotalBytes = int64(100 << 20)
	defaultMaxFileBytes  = int64(10 << 20)

	errSnapshotPathEscapesWorkspace = "snapshot file path escapes workspace: %s"
)

// Config controls snapshot workspace extraction limits.
type Config struct {
	TempDir       string
	MaxFiles      int
	MaxTotalBytes int64
	MaxFileBytes  int64
}

// Provider materializes snapshot workspace archives into per-run temp dirs.
type Provider struct {
	store         outbound.SnapshotStore
	tempDir       string
	maxFiles      int
	maxTotalBytes int64
	maxFileBytes  int64
}

var _ outbound.WorkspaceProvider = (*Provider)(nil)

// NewProvider creates a snapshot workspace provider.
func NewProvider(store outbound.SnapshotStore, cfg Config) (*Provider, error) {
	if store == nil {
		return nil, errors.New("snapshot store is required")
	}

	maxFiles := cfg.MaxFiles
	if maxFiles <= 0 {
		maxFiles = defaultMaxFiles
	}
	maxTotalBytes := cfg.MaxTotalBytes
	if maxTotalBytes <= 0 {
		maxTotalBytes = defaultMaxTotalBytes
	}
	maxFileBytes := cfg.MaxFileBytes
	if maxFileBytes <= 0 {
		maxFileBytes = defaultMaxFileBytes
	}

	return &Provider{
		store:         store,
		tempDir:       cfg.TempDir,
		maxFiles:      maxFiles,
		maxTotalBytes: maxTotalBytes,
		maxFileBytes:  maxFileBytes,
	}, nil
}

// ResolveWorkspace resolves supported workspace sources.
func (p *Provider) ResolveWorkspace(ctx context.Context, request outbound.WorkspaceResolveRequest) (outbound.Workspace, error) {
	switch request.Source.EffectiveType() {
	case run.WorkspaceSourceNone:
		return outbound.Workspace{}, nil
	case run.WorkspaceSourceSnapshot:
		return p.resolveSnapshot(ctx, request.Source.SnapshotID)
	default:
		return outbound.Workspace{}, run.ErrUnknownWorkspaceSource
	}
}

// CleanupWorkspace removes materialized snapshot workspaces.
func (p *Provider) CleanupWorkspace(_ context.Context, workspace outbound.Workspace) error {
	if strings.TrimSpace(workspace.Path) == "" {
		return nil
	}

	return os.RemoveAll(workspace.Path)
}

func (p *Provider) resolveSnapshot(ctx context.Context, snapshotID string) (outbound.Workspace, error) {
	reader, err := p.store.OpenSnapshot(ctx, outbound.SnapshotOpenRequest{SnapshotID: snapshotID})
	if err != nil {
		return outbound.Workspace{}, err
	}
	defer func() {
		_ = reader.Close()
	}()

	tempFile, err := os.CreateTemp(p.tempDir, "agentpool-snapshot-*.zip")
	if err != nil {
		return outbound.Workspace{}, fmt.Errorf("create snapshot temp file: %w", err)
	}
	tempFilePath := tempFile.Name()
	defer func() {
		_ = os.Remove(tempFilePath)
	}()

	if _, err := io.Copy(tempFile, reader); err != nil {
		_ = tempFile.Close()
		return outbound.Workspace{}, fmt.Errorf("copy snapshot archive: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return outbound.Workspace{}, fmt.Errorf("close snapshot archive: %w", err)
	}

	workspacePath, err := os.MkdirTemp(p.tempDir, "agentpool-workspace-*")
	if err != nil {
		return outbound.Workspace{}, fmt.Errorf("create workspace temp dir: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(workspacePath)
		}
	}()

	if err := p.extractZip(tempFilePath, workspacePath); err != nil {
		return outbound.Workspace{}, err
	}

	cleanup = false
	return outbound.Workspace{Path: workspacePath}, nil
}

func (p *Provider) extractZip(archivePath string, workspacePath string) error {
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open snapshot archive: %w", err)
	}
	defer func() {
		_ = archive.Close()
	}()

	fileCount := 0
	totalBytes := int64(0)
	for _, file := range archive.File {
		if file.FileInfo().IsDir() {
			continue
		}
		fileCount++
		if fileCount > p.maxFiles {
			return fmt.Errorf("snapshot exceeds %d files", p.maxFiles)
		}
		if file.UncompressedSize64 > int64LimitAsUint64(p.maxFileBytes) {
			return fmt.Errorf("snapshot file %q exceeds %d bytes", file.Name, p.maxFileBytes)
		}
		totalBytes += limitedUint64ToInt64(file.UncompressedSize64)
		if totalBytes > p.maxTotalBytes {
			return fmt.Errorf("snapshot exceeds %d bytes", p.maxTotalBytes)
		}
		if err := validateZipFile(file); err != nil {
			return err
		}
		if err := extractZipFile(file, workspacePath, p.maxFileBytes); err != nil {
			return err
		}
	}

	return nil
}

func validateZipFile(file *zip.File) error {
	name := filepath.Clean(filepath.FromSlash(file.Name))
	if name == "." || filepath.IsAbs(file.Name) || strings.HasPrefix(name, ".."+string(os.PathSeparator)) || name == ".." {
		return fmt.Errorf(errSnapshotPathEscapesWorkspace, file.Name)
	}
	mode := file.FileInfo().Mode()
	if !mode.IsRegular() {
		return fmt.Errorf("snapshot file is not a regular file: %s", file.Name)
	}

	return nil
}

func extractZipFile(file *zip.File, workspacePath string, maxFileBytes int64) error {
	targetPath, err := safeJoin(workspacePath, file.Name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o700); err != nil {
		return fmt.Errorf("create snapshot directory: %w", err)
	}

	source, err := file.Open()
	if err != nil {
		return fmt.Errorf("open snapshot file: %w", err)
	}
	defer func() {
		_ = source.Close()
	}()

	target, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create snapshot file: %w", err)
	}
	defer func() {
		_ = target.Close()
	}()

	written, err := io.Copy(target, io.LimitReader(source, maxFileBytes+1))
	if err != nil {
		return fmt.Errorf("write snapshot file: %w", err)
	}
	if written > maxFileBytes {
		return fmt.Errorf("snapshot file %q exceeds %d bytes", file.Name, maxFileBytes)
	}
	if err := target.Close(); err != nil {
		return fmt.Errorf("close snapshot file: %w", err)
	}

	return nil
}

func int64LimitAsUint64(value int64) uint64 {
	if value <= 0 {
		return 0
	}

	return uint64(value) // #nosec G115 -- value is guarded above and configured limits are int64.
}

func limitedUint64ToInt64(value uint64) int64 {
	return int64(value) // #nosec G115 -- caller checks the zip entry against an int64 max before conversion.
}

func safeJoin(root string, name string) (string, error) {
	cleanName := filepath.Clean(filepath.FromSlash(name))
	if cleanName == "." || cleanName == ".." || strings.HasPrefix(cleanName, ".."+string(os.PathSeparator)) || filepath.IsAbs(name) {
		return "", fmt.Errorf(errSnapshotPathEscapesWorkspace, name)
	}

	target := filepath.Join(root, cleanName)
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", fmt.Errorf(errSnapshotPathEscapesWorkspace, name)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf(errSnapshotPathEscapesWorkspace, name)
	}

	return target, nil
}
