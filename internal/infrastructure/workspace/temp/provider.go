package temp

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
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

	inputPath := filepath.Join(root, "input")
	workPath := filepath.Join(root, "work")
	if err := createWorkspaceDir(inputPath); err != nil {
		_ = os.RemoveAll(root)

		return outbound.Workspace{}, err
	}
	if err := createWorkspaceDir(workPath); err != nil {
		_ = os.RemoveAll(root)

		return outbound.Workspace{}, err
	}

	for _, attachment := range request.Attachments {
		if err := writeAttachment(inputPath, attachment.Filename, attachment.Content); err != nil {
			_ = os.RemoveAll(root)

			return outbound.Workspace{}, err
		}
	}

	return outbound.Workspace{
		RootPath:  root,
		InputPath: inputPath,
		WorkPath:  workPath,
		HasFiles:  len(request.Attachments) > 0,
	}, nil
}

// CleanupWorkspace removes a prepared temp workspace.
func (p *Provider) CleanupWorkspace(_ context.Context, workspace outbound.Workspace) error {
	if workspace.RootPath == "" {
		return nil
	}

	return os.RemoveAll(workspace.RootPath)
}

// CollectArtifacts captures bounded regular files from /workspace/work before cleanup.
func (p *Provider) CollectArtifacts(ctx context.Context, workspace outbound.Workspace) ([]run.Artifact, error) {
	ok, err := workspaceWorkPathAvailable(workspace.WorkPath)
	if err != nil || !ok {
		return nil, err
	}

	collector := artifactCollector{root: workspace.WorkPath}
	err = filepath.WalkDir(workspace.WorkPath, func(current string, entry os.DirEntry, err error) error {
		return collector.addEntry(ctx, current, entry, err)
	})
	if err != nil {
		return nil, err
	}

	return collector.artifacts, nil
}

func workspaceWorkPathAvailable(workPath string) (bool, error) {
	if strings.TrimSpace(workPath) == "" {
		return false, nil
	}
	if _, err := os.Stat(workPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

func createWorkspaceDir(path string) error {
	if err := os.Mkdir(path, workspaceDirPerm); err != nil {
		return err
	}

	return os.Chmod(path, workspaceDirPerm)
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

type artifactCollector struct {
	root      string
	artifacts []run.Artifact
	totalSize int64
}

func (c *artifactCollector) addEntry(ctx context.Context, current string, entry os.DirEntry, walkErr error) error {
	if walkErr != nil {
		return walkErr
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if current == c.root || entry.IsDir() {
		return nil
	}
	if entry.Type()&os.ModeSymlink != 0 {
		return nil
	}
	info, err := entry.Info()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return nil
	}

	return c.add(current, info.Size())
}

func (c *artifactCollector) add(pathValue string, size int64) error {
	if len(c.artifacts) >= run.MaxArtifactCount ||
		size < 0 ||
		size > run.MaxArtifactSizeBytes ||
		c.totalSize+size > run.MaxTotalArtifactSizeBytes {
		return nil
	}

	relative, err := filepath.Rel(c.root, pathValue)
	if err != nil {
		return err
	}
	artifactPath := filepath.ToSlash(relative)
	if !safeArtifactPath(artifactPath) {
		return nil
	}

	content, err := readArtifactFile(pathValue, size)
	if err != nil {
		return err
	}
	c.artifacts = append(c.artifacts, run.Artifact{
		Path:      artifactPath,
		MediaType: artifactMediaType(artifactPath, content),
		Content:   content,
		SizeBytes: int64(len(content)),
	})
	c.totalSize += int64(len(content))

	return nil
}

func safeArtifactPath(path string) bool {
	return run.ValidateArtifactPath(path) == nil
}

func readArtifactFile(pathValue string, size int64) ([]byte, error) {
	source, err := os.Open(pathValue) //nolint:gosec // workspace path is created by the workspace provider.
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = source.Close()
	}()

	content, err := io.ReadAll(io.LimitReader(source, run.MaxArtifactSizeBytes+1))
	if err != nil {
		return nil, err
	}
	if len(content) > int(run.MaxArtifactSizeBytes) ||
		(size > 0 && int64(len(content)) != size) {
		return nil, errors.New("artifact file size changed while reading")
	}

	return content, nil
}

func artifactMediaType(artifactPath string, content []byte) string {
	switch strings.ToLower(path.Ext(artifactPath)) {
	case ".md":
		return "text/markdown; charset=utf-8"
	case ".txt", ".go", ".py", ".js", ".ts", ".tsx", ".jsx", ".sh", ".toml", ".yaml", ".yml", ".mod", ".sum":
		return "text/plain; charset=utf-8"
	}
	if mediaType := mime.TypeByExtension(path.Ext(artifactPath)); mediaType != "" {
		return mediaType
	}
	if len(content) > 0 {
		return http.DetectContentType(content)
	}

	return "application/octet-stream"
}
