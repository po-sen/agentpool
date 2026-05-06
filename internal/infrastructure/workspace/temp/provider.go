package temp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

const (
	workspaceDirPerm  = 0o700
	workspaceFilePerm = 0o600

	workspaceDirName = "workspace"
	stateDirName     = "state"
	sourcesDirName   = "sources"
	stagedFileName   = "staged.json"
)

// Config controls temp workspace creation.
type Config struct {
	BaseDir string
}

// Provider prepares ephemeral mutable workspaces backed by authorized input sources.
type Provider struct {
	baseDir string
}

var _ outbound.WorkspaceProvider = (*Provider)(nil)
var _ outbound.WorkspaceMaterializer = (*Provider)(nil)

// NewProvider creates a temp workspace provider.
func NewProvider(cfg Config) *Provider {
	return &Provider{baseDir: cfg.BaseDir}
}

// PrepareWorkspace stores run attachments as authorized sources and creates an empty workspace.
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

	workspacePath := filepath.Join(root, workspaceDirName)
	statePath := filepath.Join(root, stateDirName)
	sourcesPath := filepath.Join(statePath, sourcesDirName)
	if err := createWorkspaceDir(workspacePath); err != nil {
		_ = os.RemoveAll(root)

		return outbound.Workspace{}, err
	}
	if err := createWorkspaceDir(statePath); err != nil {
		_ = os.RemoveAll(root)

		return outbound.Workspace{}, err
	}
	if err := createWorkspaceDir(sourcesPath); err != nil {
		_ = os.RemoveAll(root)

		return outbound.Workspace{}, err
	}

	sources := make([]outbound.WorkspaceSource, 0, len(request.Attachments))
	for index, attachment := range request.Attachments {
		source, err := workspaceSourceFromAttachment(index, attachment)
		if err != nil {
			_ = os.RemoveAll(root)

			return outbound.Workspace{}, err
		}
		if err := writeSource(sourcesPath, source.ID, attachment.Content); err != nil {
			_ = os.RemoveAll(root)

			return outbound.Workspace{}, err
		}
		sources = append(sources, source)
	}

	return outbound.Workspace{
		RootPath:  workspacePath,
		StatePath: statePath,
		Sources:   sources,
	}, nil
}

// CleanupWorkspace removes a prepared temp workspace.
func (p *Provider) CleanupWorkspace(_ context.Context, workspace outbound.Workspace) error {
	if workspace.RootPath == "" && workspace.StatePath == "" {
		return nil
	}
	if workspace.RootPath != "" && workspace.StatePath != "" {
		rootParent := filepath.Dir(workspace.RootPath)
		stateParent := filepath.Dir(workspace.StatePath)
		if rootParent == stateParent {
			return os.RemoveAll(rootParent)
		}
	}

	rootErr := os.RemoveAll(workspace.RootPath)
	stateErr := os.RemoveAll(workspace.StatePath)
	if rootErr != nil {
		return rootErr
	}

	return stateErr
}

// MaterializeWorkspaceSource copies an authorized source into the mutable workspace.
func (p *Provider) MaterializeWorkspaceSource(
	_ context.Context,
	request outbound.WorkspaceMaterializeRequest,
) (outbound.WorkspaceStagedFile, error) {
	if err := validateWorkspacePaths(request.Workspace); err != nil {
		return outbound.WorkspaceStagedFile{}, err
	}
	source, ok := findWorkspaceSource(request.Workspace.Sources, request.SourceID)
	if !ok {
		return outbound.WorkspaceStagedFile{}, errors.New("workspace source is not available")
	}

	targetPath := strings.TrimSpace(request.Path)
	if targetPath == "" {
		targetPath = source.Path
	}
	relativePath, err := safeRelativeWorkspacePath(targetPath)
	if err != nil {
		return outbound.WorkspaceStagedFile{}, err
	}
	target, err := confinedPath(request.Workspace.RootPath, relativePath)
	if err != nil {
		return outbound.WorkspaceStagedFile{}, err
	}
	if err := prepareMaterializeTarget(request.Workspace.RootPath, relativePath, request.Overwrite); err != nil {
		return outbound.WorkspaceStagedFile{}, err
	}

	sourcePath := sourceStoragePath(request.Workspace.StatePath, source.ID)
	if err := copyFile(sourcePath, target); err != nil {
		return outbound.WorkspaceStagedFile{}, err
	}
	staged := outbound.WorkspaceStagedFile{
		SourceID:  source.ID,
		Path:      relativePath,
		SizeBytes: source.SizeBytes,
		Checksum:  source.Checksum,
	}
	if err := p.recordStagedFile(request.Workspace, staged); err != nil {
		return outbound.WorkspaceStagedFile{}, err
	}

	return staged, nil
}

// ListWorkspaceStagedFiles returns source mappings for files materialized into the workspace.
func (p *Provider) ListWorkspaceStagedFiles(
	_ context.Context,
	workspace outbound.Workspace,
) ([]outbound.WorkspaceStagedFile, error) {
	if strings.TrimSpace(workspace.StatePath) == "" {
		return nil, nil
	}

	manifest, err := readStagedManifest(stagedManifestPath(workspace.StatePath))
	if err != nil {
		return nil, err
	}

	return manifest.Files, nil
}

// CollectArtifacts captures bounded changed or newly-created regular files from /workspace before cleanup.
func (p *Provider) CollectArtifacts(ctx context.Context, workspace outbound.Workspace) ([]run.Artifact, error) {
	ok, err := workspaceRootPathAvailable(workspace.RootPath)
	if err != nil || !ok {
		return nil, err
	}
	stagedFiles, err := p.ListWorkspaceStagedFiles(ctx, workspace)
	if err != nil {
		return nil, err
	}

	collector := artifactCollector{root: workspace.RootPath, stagedFiles: stagedFileMap(stagedFiles)}
	err = filepath.WalkDir(workspace.RootPath, func(current string, entry os.DirEntry, err error) error {
		return collector.addEntry(ctx, current, entry, err)
	})
	if err != nil {
		return nil, err
	}

	return collector.artifacts, nil
}

func workspaceRootPathAvailable(rootPath string) (bool, error) {
	if strings.TrimSpace(rootPath) == "" {
		return false, nil
	}
	if _, err := os.Stat(rootPath); err != nil {
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

func workspaceSourceFromAttachment(index int, attachment run.TaskAttachment) (outbound.WorkspaceSource, error) {
	sourcePath, err := safeRelativeWorkspacePath(attachment.Filename)
	if err != nil {
		return outbound.WorkspaceSource{}, err
	}

	return outbound.WorkspaceSource{
		ID:        fmtSourceID(index),
		Path:      sourcePath,
		MediaType: attachment.MediaType,
		SizeBytes: attachmentSizeBytes(attachment),
		Checksum:  checksumBytes(attachment.Content),
	}, nil
}

func fmtSourceID(index int) string {
	return "input_" + leftPadInt(index+1, 3)
}

func leftPadInt(value int, width int) string {
	raw := strconv.Itoa(value)
	if len(raw) >= width {
		return raw
	}

	return strings.Repeat("0", width-len(raw)) + raw
}

func attachmentSizeBytes(attachment run.TaskAttachment) int64 {
	if attachment.SizeBytes > 0 {
		return attachment.SizeBytes
	}

	return int64(len(attachment.Content))
}

func checksumBytes(content []byte) string {
	sum := sha256.Sum256(content)

	return "sha256:" + hex.EncodeToString(sum[:])
}

func writeSource(stateSourcesPath string, sourceID string, content []byte) error {
	target := sourceStoragePath(filepath.Dir(stateSourcesPath), sourceID)
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

func sourceStoragePath(statePath string, sourceID string) string {
	return filepath.Join(statePath, sourcesDirName, sourceID)
}

func copyFile(sourcePath string, targetPath string) error {
	source, err := os.Open(sourcePath) //nolint:gosec // source path is provider-owned workspace state.
	if err != nil {
		return err
	}
	defer func() {
		_ = source.Close()
	}()

	target, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, workspaceFilePerm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(target, source); err != nil {
		_ = target.Close()

		return err
	}

	return target.Close()
}

func findWorkspaceSource(sources []outbound.WorkspaceSource, sourceID string) (outbound.WorkspaceSource, bool) {
	sourceID = strings.TrimSpace(sourceID)
	for _, source := range sources {
		if source.ID == sourceID {
			return source, true
		}
	}

	return outbound.WorkspaceSource{}, false
}

func validateWorkspacePaths(workspace outbound.Workspace) error {
	if strings.TrimSpace(workspace.RootPath) == "" {
		return errors.New("workspace root path is required")
	}
	if strings.TrimSpace(workspace.StatePath) == "" {
		return errors.New("workspace state path is required")
	}

	return nil
}

func prepareMaterializeTarget(root string, relativePath string, overwrite bool) error {
	if err := ensureWorkspaceParent(root, relativePath); err != nil {
		return err
	}

	target, err := confinedPath(root, relativePath)
	if err != nil {
		return err
	}
	info, err := os.Lstat(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return err
	}
	if !overwrite {
		return errors.New("workspace path already exists; use restore to overwrite")
	}
	if info.IsDir() {
		return errors.New("workspace path already exists as directory")
	}

	return os.Remove(target)
}

func ensureWorkspaceParent(root string, relativePath string) error {
	parent := path.Dir(relativePath)
	if parent == "." {
		return nil
	}

	current := root
	for _, component := range strings.Split(parent, "/") {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				if err := os.Mkdir(current, workspaceDirPerm); err != nil {
					return err
				}

				continue
			}

			return err
		}
		if !info.IsDir() {
			return errors.New("workspace parent path is not a directory")
		}
	}

	return nil
}

func confinedPath(root string, filename string) (string, error) {
	relativePath, err := safeRelativeWorkspacePath(filename)
	if err != nil {
		return "", err
	}

	target := filepath.Join(root, filepath.FromSlash(relativePath))
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return "", errors.New("workspace file escapes root")
	}

	return target, nil
}

func safeRelativeWorkspacePath(filename string) (string, error) {
	cleanPath := strings.TrimPrefix(strings.TrimSpace(filename), "/workspace/")
	if cleanPath == "" ||
		cleanPath == "/workspace" ||
		path.IsAbs(cleanPath) ||
		filepath.IsAbs(cleanPath) ||
		filepath.VolumeName(cleanPath) != "" ||
		strings.Contains(cleanPath, "\\") ||
		strings.Contains(cleanPath, ":") ||
		path.Clean(cleanPath) != cleanPath {
		return "", errors.New("unsafe workspace file path")
	}
	for _, component := range strings.Split(cleanPath, "/") {
		if component == "" || component == "." || component == ".." {
			return "", errors.New("unsafe workspace file path")
		}
	}

	return cleanPath, nil
}

type stagedManifest struct {
	Files []outbound.WorkspaceStagedFile `json:"files"`
}

func (p *Provider) recordStagedFile(workspace outbound.Workspace, staged outbound.WorkspaceStagedFile) error {
	manifestPath := stagedManifestPath(workspace.StatePath)
	manifest, err := readStagedManifest(manifestPath)
	if err != nil {
		return err
	}
	updated := false
	for index, file := range manifest.Files {
		if file.Path == staged.Path {
			manifest.Files[index] = staged
			updated = true

			break
		}
	}
	if !updated {
		manifest.Files = append(manifest.Files, staged)
	}

	return writeStagedManifest(manifestPath, manifest)
}

func stagedManifestPath(statePath string) string {
	return filepath.Join(statePath, stagedFileName)
}

func readStagedManifest(manifestPath string) (stagedManifest, error) {
	content, err := os.ReadFile(manifestPath) //nolint:gosec // manifest path is provider-owned workspace state.
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return stagedManifest{}, nil
		}

		return stagedManifest{}, err
	}
	if len(content) == 0 {
		return stagedManifest{}, nil
	}

	var manifest stagedManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return stagedManifest{}, err
	}

	return manifest, nil
}

func writeStagedManifest(manifestPath string, manifest stagedManifest) error {
	if err := os.MkdirAll(filepath.Dir(manifestPath), workspaceDirPerm); err != nil {
		return err
	}
	content, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')

	return os.WriteFile(manifestPath, content, workspaceFilePerm)
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
	root        string
	stagedFiles map[string]outbound.WorkspaceStagedFile
	artifacts   []run.Artifact
	totalSize   int64
}

func (c *artifactCollector) addEntry(ctx context.Context, current string, entry os.DirEntry, walkErr error) error {
	if walkErr != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		return nil
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
	info, ok := artifactEntryInfo(entry)
	if !ok {
		return nil
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	if c.unchangedStagedFile(current) {
		return nil
	}

	return c.add(current, info.Size())
}

func (c *artifactCollector) unchangedStagedFile(pathValue string) bool {
	relative, err := filepath.Rel(c.root, pathValue)
	if err != nil {
		return false
	}
	record, ok := c.stagedFiles[filepath.ToSlash(relative)]
	if !ok || record.Checksum == "" {
		return false
	}
	checksum, err := checksumFile(pathValue)
	if err != nil {
		return false
	}

	return checksum == record.Checksum
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

	content, ok := artifactFileContent(pathValue, size)
	if !ok {
		return nil
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

func artifactEntryInfo(entry os.DirEntry) (os.FileInfo, bool) {
	info, err := entry.Info()
	if err != nil {
		return nil, false
	}

	return info, true
}

func safeArtifactPath(path string) bool {
	return run.ValidateArtifactPath(path) == nil
}

func artifactFileContent(pathValue string, size int64) ([]byte, bool) {
	content, err := readArtifactFile(pathValue, size)
	if err != nil {
		return nil, false
	}

	return content, true
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

func stagedFileMap(files []outbound.WorkspaceStagedFile) map[string]outbound.WorkspaceStagedFile {
	if len(files) == 0 {
		return nil
	}
	index := make(map[string]outbound.WorkspaceStagedFile, len(files))
	for _, file := range files {
		index[file.Path] = file
	}

	return index
}

func checksumFile(pathValue string) (string, error) {
	file, err := os.Open(pathValue) //nolint:gosec // workspace file path was confined by the provider.
	if err != nil {
		return "", err
	}
	defer func() {
		_ = file.Close()
	}()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}
