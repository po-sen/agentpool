package temp

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

const (
	workspaceDirPerm  = 0o700
	workspaceFilePerm = 0o600
)

// Config controls temp workspace creation.
type Config struct {
	BaseDir string
}

// Provider materializes run attachments into ephemeral temp workspaces.
type Provider struct {
	baseDir string
}

var _ outbound.WorkspaceProvider = (*Provider)(nil)

// NewProvider creates a temp workspace provider.
func NewProvider(cfg Config) *Provider {
	return &Provider{baseDir: cfg.BaseDir}
}

// PrepareWorkspace writes run attachments into a per-run temp directory.
func (p *Provider) PrepareWorkspace(
	_ context.Context,
	request outbound.WorkspacePrepareRequest,
) (outbound.Workspace, error) {
	root, err := os.MkdirTemp(p.baseDir, "agentpool-run-"+safeRunID(request.RunID.String())+"-")
	if err != nil {
		return outbound.Workspace{}, err
	}
	if err := os.Chmod(root, workspaceDirPerm); err != nil {
		_ = os.RemoveAll(root)

		return outbound.Workspace{}, err
	}

	for _, attachment := range request.Attachments {
		if err := writeAttachment(root, attachment.Filename, attachment.Content); err != nil {
			_ = os.RemoveAll(root)

			return outbound.Workspace{}, err
		}
	}

	return outbound.Workspace{Path: root, HasFiles: len(request.Attachments) > 0}, nil
}

// CleanupWorkspace removes a prepared temp workspace.
func (p *Provider) CleanupWorkspace(_ context.Context, workspace outbound.Workspace) error {
	if workspace.Path == "" {
		return nil
	}

	return os.RemoveAll(workspace.Path)
}

func writeAttachment(root string, filename string, content []byte) error {
	target, err := confinedPath(root, filename)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), workspaceDirPerm); err != nil {
		return err
	}

	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, workspaceFilePerm)
	if err != nil {
		return err
	}

	if _, err := file.Write(content); err != nil {
		_ = file.Close()

		return err
	}

	return file.Close()
}

func confinedPath(root string, filename string) (string, error) {
	if filename == "" ||
		path.IsAbs(filename) ||
		filepath.IsAbs(filename) ||
		strings.Contains(filename, "\\") ||
		strings.Contains(filename, ":") ||
		path.Clean(filename) != filename {
		return "", errors.New("unsafe workspace file path")
	}
	for _, component := range strings.Split(filename, "/") {
		if component == "" || component == "." || component == ".." {
			return "", errors.New("unsafe workspace file path")
		}
	}

	target := filepath.Join(root, filepath.FromSlash(filename))
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return "", errors.New("workspace file escapes root")
	}

	return target, nil
}

func safeRunID(id string) string {
	var builder strings.Builder
	for _, char := range id {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '-' ||
			char == '_' {
			builder.WriteRune(char)
		}
	}
	if builder.Len() == 0 {
		return "run"
	}

	return builder.String()
}
