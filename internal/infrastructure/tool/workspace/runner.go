package workspace

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

const (
	toolNameWorkspace = "workspace"

	argumentOperation = "operation"
	argumentSourceID  = "source_id"
	argumentSourceIDs = "source_ids"
	argumentPath      = "path"

	operationListSources = "list_sources"
	operationStage       = "stage"
	operationStageMany   = "stage_many"
	operationRestore     = "restore"
	operationList        = "list"

	defaultMaxFiles = 500

	virtualWorkspaceRoot = "/workspace"
	messagePathUnsafe    = "path is unsafe"

	formatSourceIDLine = "- source_id: "
	formatIndentedPath = "\n  path: "
	formatIndentedSize = "\n  size_bytes: "
)

var errTooManyFiles = errors.New("too many files")

// Config controls workspace control-plane tool limits.
type Config struct {
	MaxFiles int
}

// Runner exposes workspace source staging and metadata operations.
type Runner struct {
	materializer outbound.WorkspaceMaterializer
	maxFiles     int
}

var _ outbound.ToolRunner = (*Runner)(nil)

// NewRunner creates a workspace control-plane tool runner.
func NewRunner(materializer outbound.WorkspaceMaterializer, cfg Config) (*Runner, error) {
	if materializer == nil {
		return nil, errors.New("workspace materializer is required")
	}

	maxFiles := cfg.MaxFiles
	if maxFiles <= 0 {
		maxFiles = defaultMaxFiles
	}

	return &Runner{materializer: materializer, maxFiles: maxFiles}, nil
}

// ListTools exposes workspace when a structured workspace is available.
func (r *Runner) ListTools(_ context.Context, request outbound.ToolListRequest) ([]outbound.ToolDefinition, error) {
	if !workspaceAvailable(request.Context.Workspace) {
		return nil, nil
	}
	if len(request.Context.Workspace.Sources) == 0 {
		return nil, nil
	}

	return []outbound.ToolDefinition{
		{
			Name:        toolNameWorkspace,
			Description: "Manages authorized input sources and staged files for the mutable /workspace.",
			Arguments: []outbound.ToolArgumentDefinition{
				{
					Name:        argumentOperation,
					Description: `Operation to run. Supported values: "list_sources", "stage", "stage_many", "restore", or "list".`,
					Required:    true,
					Example:     "list_sources",
				},
				{
					Name:        argumentSourceID,
					Description: `Authorized source id returned by workspace context or list_sources. Required for "stage"; optional for "restore" when path identifies a staged file.`,
					Required:    false,
					Example:     "input_001",
				},
				{
					Name:        argumentSourceIDs,
					Description: `Comma-separated authorized source ids returned by workspace context or list_sources to stage with "stage_many".`,
					Required:    false,
					Example:     "input_001,input_002",
				},
				{
					Name:        argumentPath,
					Description: `Safe relative /workspace path, or a full /workspace virtual path. Defaults to the source path for stage and restore, and "." for list. Not used by "stage_many".`,
					Required:    false,
					Example:     "README.md",
				},
			},
		},
	}, nil
}

// RunTool executes a workspace control-plane operation.
func (r *Runner) RunTool(ctx context.Context, call outbound.ToolCall) (outbound.ToolResult, error) {
	if call.Name != toolNameWorkspace {
		return outbound.ToolResult{Content: fmt.Sprintf("unknown tool: %s", call.Name), IsError: true}, nil
	}
	if !workspaceAvailable(call.Context.Workspace) {
		return outbound.ToolResult{Content: "workspace is not available", IsError: true}, nil
	}

	operation := strings.TrimSpace(call.Arguments[argumentOperation])
	if operation == "" {
		return outbound.ToolResult{Content: "missing operation argument", IsError: true}, nil
	}

	switch operation {
	case operationListSources:
		return r.listSources(ctx, call.Context.Workspace), nil
	case operationStage:
		return r.stage(ctx, call.Context.Workspace, call.Arguments), nil
	case operationStageMany:
		return r.stageMany(ctx, call.Context.Workspace, call.Arguments), nil
	case operationRestore:
		return r.restore(ctx, call.Context.Workspace, call.Arguments), nil
	case operationList:
		return r.list(ctx, call.Context.Workspace, call.Arguments[argumentPath]), nil
	default:
		return outbound.ToolResult{Content: fmt.Sprintf("unknown workspace operation: %s", operation), IsError: true}, nil
	}
}

func workspaceAvailable(workspace outbound.Workspace) bool {
	return strings.TrimSpace(workspace.RootPath) != ""
}

func (r *Runner) listSources(ctx context.Context, workspace outbound.Workspace) outbound.ToolResult {
	stagedFiles, err := r.materializer.ListWorkspaceStagedFiles(ctx, workspace)
	if err != nil {
		return outbound.ToolResult{Content: "list workspace sources failed", IsError: true}
	}
	stagedPathsBySource := stagedPathIndex(stagedFiles)

	if len(workspace.Sources) == 0 {
		return outbound.ToolResult{Content: "sources:\n"}
	}

	var builder strings.Builder
	builder.WriteString("sources:\n")
	for _, source := range workspace.Sources {
		builder.WriteString(formatSourceIDLine)
		builder.WriteString(source.ID)
		builder.WriteString(formatIndentedPath)
		builder.WriteString(source.Path)
		builder.WriteString("\n  virtual_path: ")
		builder.WriteString(virtualPath(source.Path))
		if source.MediaType != "" {
			builder.WriteString("\n  media_type: ")
			builder.WriteString(source.MediaType)
		}
		builder.WriteString(formatIndentedSize)
		_, _ = fmt.Fprintf(&builder, "%d", source.SizeBytes)
		if source.Checksum != "" {
			builder.WriteString("\n  checksum: ")
			builder.WriteString(source.Checksum)
		}
		if stagedPaths := stagedPathsBySource[source.ID]; len(stagedPaths) > 0 {
			builder.WriteString("\n  staged: true")
			builder.WriteString("\n  staged_paths: ")
			builder.WriteString(strings.Join(stagedPaths, ", "))
		} else {
			builder.WriteString("\n  staged: false")
		}
		builder.WriteString("\n")
	}

	return outbound.ToolResult{Content: builder.String()}
}

func (r *Runner) stage(
	ctx context.Context,
	workspace outbound.Workspace,
	arguments map[string]string,
) outbound.ToolResult {
	sourceID := strings.TrimSpace(arguments[argumentSourceID])
	if sourceID == "" {
		return outbound.ToolResult{Content: "missing source_id argument", IsError: true}
	}
	source, ok := findSource(workspace.Sources, sourceID)
	if !ok {
		return outbound.ToolResult{Content: "workspace source is not available", IsError: true}
	}
	targetPath, pathMessage := stagePath(arguments[argumentPath], source.Path)
	if pathMessage != "" {
		return outbound.ToolResult{Content: pathMessage, IsError: true}
	}

	staged, err := r.materializer.MaterializeWorkspaceSource(ctx, outbound.WorkspaceMaterializeRequest{
		Workspace: workspace,
		SourceID:  source.ID,
		Path:      targetPath,
		Overwrite: false,
	})
	if err != nil {
		return outbound.ToolResult{Content: safeWorkspaceError(err), IsError: true}
	}

	return outbound.ToolResult{Content: formatMaterializedFile("staged", staged)}
}

func (r *Runner) stageMany(
	ctx context.Context,
	workspace outbound.Workspace,
	arguments map[string]string,
) outbound.ToolResult {
	sourceIDs := parseSourceIDs(arguments[argumentSourceIDs])
	if len(sourceIDs) == 0 {
		return outbound.ToolResult{Content: "missing source_ids argument", IsError: true}
	}
	if len(sourceIDs) > r.maxFiles {
		return outbound.ToolResult{Content: "too many source_ids", IsError: true}
	}

	sources, message := findSources(workspace.Sources, sourceIDs)
	if message != "" {
		return outbound.ToolResult{Content: message, IsError: true}
	}

	stagedFiles := make([]outbound.WorkspaceStagedFile, 0, len(sources))
	failures := make([]workspaceSourceFailure, 0)
	for _, source := range sources {
		staged, err := r.materializer.MaterializeWorkspaceSource(ctx, outbound.WorkspaceMaterializeRequest{
			Workspace: workspace,
			SourceID:  source.ID,
			Path:      source.Path,
			Overwrite: false,
		})
		if err != nil {
			failures = append(failures, workspaceSourceFailure{
				sourceID: source.ID,
				message:  safeWorkspaceError(err),
			})

			continue
		}
		stagedFiles = append(stagedFiles, staged)
	}

	return outbound.ToolResult{
		Content: formatMaterializedFiles("staged", stagedFiles, failures),
		IsError: len(failures) > 0,
	}
}

func (r *Runner) restore(
	ctx context.Context,
	workspace outbound.Workspace,
	arguments map[string]string,
) outbound.ToolResult {
	sourceID := strings.TrimSpace(arguments[argumentSourceID])
	rawPath := strings.TrimSpace(arguments[argumentPath])
	if sourceID == "" && rawPath == "" {
		return outbound.ToolResult{Content: "missing source_id or path argument", IsError: true}
	}

	stagedFiles, err := r.materializer.ListWorkspaceStagedFiles(ctx, workspace)
	if err != nil {
		return outbound.ToolResult{Content: "list staged workspace files failed", IsError: true}
	}

	targetPath, pathMessage := restorePath(rawPath)
	if pathMessage != "" {
		return outbound.ToolResult{Content: pathMessage, IsError: true}
	}
	if sourceID == "" {
		staged, ok := findStagedPath(stagedFiles, targetPath)
		if !ok {
			return outbound.ToolResult{Content: "workspace path has no staged source", IsError: true}
		}
		sourceID = staged.SourceID
	}
	source, ok := findSource(workspace.Sources, sourceID)
	if !ok {
		return outbound.ToolResult{Content: "workspace source is not available", IsError: true}
	}
	if targetPath == "" {
		targetPath = source.Path
	}

	staged, err := r.materializer.MaterializeWorkspaceSource(ctx, outbound.WorkspaceMaterializeRequest{
		Workspace: workspace,
		SourceID:  source.ID,
		Path:      targetPath,
		Overwrite: true,
	})
	if err != nil {
		return outbound.ToolResult{Content: safeWorkspaceError(err), IsError: true}
	}

	return outbound.ToolResult{Content: formatMaterializedFile("restored", staged)}
}

func (r *Runner) list(ctx context.Context, workspace outbound.Workspace, rawPath string) outbound.ToolResult {
	relativePath, pathMessage := listPath(rawPath)
	if pathMessage != "" {
		return outbound.ToolResult{Content: pathMessage, IsError: true}
	}
	stagedFiles, err := r.materializer.ListWorkspaceStagedFiles(ctx, workspace)
	if err != nil {
		return outbound.ToolResult{Content: "list staged workspace files failed", IsError: true}
	}

	files, foundPath, err := r.collectFiles(ctx, workspace.RootPath, relativePath)
	if errors.Is(err, errTooManyFiles) {
		return outbound.ToolResult{Content: "workspace contains too many files to list", IsError: true}
	}
	if err != nil {
		return outbound.ToolResult{Content: "list workspace failed", IsError: true}
	}
	if !foundPath {
		return outbound.ToolResult{Content: "path is not available", IsError: true}
	}
	sort.Slice(files, func(i int, j int) bool {
		return files[i].path < files[j].path
	})

	return outbound.ToolResult{Content: formatWorkspaceFiles(files, stagedFileIndex(stagedFiles))}
}

type workspaceFile struct {
	path      string
	sizeBytes int64
}

type workspaceSourceFailure struct {
	sourceID string
	message  string
}

func (r *Runner) collectFiles(
	ctx context.Context,
	rootPath string,
	relativePath string,
) ([]workspaceFile, bool, error) {
	root, err := confinedPath(rootPath, relativePath)
	if err != nil {
		return nil, false, err
	}
	if _, err := os.Lstat(root); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}

		return nil, false, err
	}

	var files []workspaceFile
	err = filepath.WalkDir(root, func(current string, entry fs.DirEntry, err error) error {
		file, ok, err := collectWorkspaceFile(ctx, rootPath, current, entry, err)
		if err != nil || !ok {
			return err
		}
		files = append(files, file)
		if len(files) > r.maxFiles {
			return errTooManyFiles
		}

		return nil
	})

	return files, true, err
}

func collectWorkspaceFile(
	ctx context.Context,
	rootPath string,
	current string,
	entry fs.DirEntry,
	walkErr error,
) (workspaceFile, bool, error) {
	if walkErr != nil {
		return workspaceFile{}, false, walkErr
	}
	if ctx.Err() != nil {
		return workspaceFile{}, false, ctx.Err()
	}
	if entry.IsDir() {
		return workspaceFile{}, false, nil
	}
	info, err := entry.Info()
	if err != nil {
		return workspaceFile{}, false, err
	}
	if !info.Mode().IsRegular() {
		return workspaceFile{}, false, nil
	}

	relative, err := filepath.Rel(rootPath, current)
	if err != nil {
		return workspaceFile{}, false, err
	}

	return workspaceFile{path: filepath.ToSlash(relative), sizeBytes: info.Size()}, true, nil
}

func formatWorkspaceFiles(
	files []workspaceFile,
	stagedFiles map[string]outbound.WorkspaceStagedFile,
) string {
	if len(files) == 0 {
		return "files:\n"
	}

	var builder strings.Builder
	builder.WriteString("files:\n")
	for _, file := range files {
		builder.WriteString("- virtual_path: ")
		builder.WriteString(virtualPath(file.path))
		builder.WriteString("\n  path: ")
		builder.WriteString(file.path)
		builder.WriteString("\n  size_bytes: ")
		_, _ = fmt.Fprintf(&builder, "%d", file.sizeBytes)
		if staged, ok := stagedFiles[file.path]; ok {
			builder.WriteString("\n  source_id: ")
			builder.WriteString(staged.SourceID)
		}
		builder.WriteString("\n")
	}

	return builder.String()
}

func formatMaterializedFile(label string, staged outbound.WorkspaceStagedFile) string {
	return formatMaterializedFiles(label, []outbound.WorkspaceStagedFile{staged}, nil)
}

func formatMaterializedFiles(
	label string,
	stagedFiles []outbound.WorkspaceStagedFile,
	failures []workspaceSourceFailure,
) string {
	var builder strings.Builder
	builder.WriteString(label)
	builder.WriteString(":\n")
	if len(stagedFiles) == 1 && len(failures) == 0 {
		builder.WriteString("source_id: ")
		builder.WriteString(stagedFiles[0].SourceID)
		builder.WriteString("\nvirtual_path: ")
		builder.WriteString(virtualPath(stagedFiles[0].Path))
		builder.WriteString("\npath: ")
		builder.WriteString(stagedFiles[0].Path)
		builder.WriteString("\nsize_bytes: ")
		_, _ = fmt.Fprintf(&builder, "%d", stagedFiles[0].SizeBytes)
		if stagedFiles[0].Checksum != "" {
			builder.WriteString("\nchecksum: ")
			builder.WriteString(stagedFiles[0].Checksum)
		}
		builder.WriteString("\n")

		return builder.String()
	}

	if len(stagedFiles) == 0 {
		builder.WriteString("none\n")
	} else {
		for _, staged := range stagedFiles {
			builder.WriteString(formatSourceIDLine)
			builder.WriteString(staged.SourceID)
			builder.WriteString("\n  virtual_path: ")
			builder.WriteString(virtualPath(staged.Path))
			builder.WriteString(formatIndentedPath)
			builder.WriteString(staged.Path)
			builder.WriteString(formatIndentedSize)
			_, _ = fmt.Fprintf(&builder, "%d", staged.SizeBytes)
			if staged.Checksum != "" {
				builder.WriteString("\n  checksum: ")
				builder.WriteString(staged.Checksum)
			}
			builder.WriteString("\n")
		}
	}
	if len(failures) > 0 {
		builder.WriteString("failed:\n")
		for _, failure := range failures {
			builder.WriteString(formatSourceIDLine)
			builder.WriteString(failure.sourceID)
			builder.WriteString("\n  error: ")
			builder.WriteString(failure.message)
			builder.WriteString("\n")
		}
	}

	builder.WriteString("\n")

	return builder.String()
}

func stagePath(rawPath string, defaultPath string) (string, string) {
	if strings.TrimSpace(rawPath) == "" {
		return safeRelativePath(defaultPath)
	}

	return safeRelativePath(rawPath)
}

func restorePath(rawPath string) (string, string) {
	if strings.TrimSpace(rawPath) == "" {
		return "", ""
	}

	return safeRelativePath(rawPath)
}

func listPath(rawPath string) (string, string) {
	if strings.TrimSpace(rawPath) == "" {
		return ".", ""
	}
	if strings.TrimSpace(rawPath) == virtualWorkspaceRoot {
		return ".", ""
	}

	return safeRelativePath(rawPath)
}

func safeRelativePath(rawPath string) (string, string) {
	cleanPath := strings.TrimSpace(rawPath)
	if strings.HasPrefix(cleanPath, virtualWorkspaceRoot+"/") {
		cleanPath = strings.TrimPrefix(cleanPath, virtualWorkspaceRoot+"/")
	}
	if cleanPath == "" ||
		cleanPath == virtualWorkspaceRoot ||
		path.IsAbs(cleanPath) ||
		filepath.IsAbs(cleanPath) ||
		filepath.VolumeName(cleanPath) != "" ||
		strings.Contains(cleanPath, "\\") ||
		strings.Contains(cleanPath, ":") ||
		path.Clean(cleanPath) != cleanPath {
		return "", messagePathUnsafe
	}
	if cleanPath == "." {
		return cleanPath, ""
	}
	for _, component := range strings.Split(cleanPath, "/") {
		if component == "" || component == "." || component == ".." {
			return "", messagePathUnsafe
		}
	}

	return cleanPath, ""
}

func confinedPath(rootPath string, relativePath string) (string, error) {
	target := rootPath
	if relativePath != "." {
		target = filepath.Join(rootPath, filepath.FromSlash(relativePath))
	}
	relative, err := filepath.Rel(rootPath, target)
	if err != nil ||
		relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) ||
		filepath.IsAbs(relative) {
		return "", errors.New("path escapes workspace")
	}

	return target, nil
}

func virtualPath(relativePath string) string {
	if relativePath == "." || relativePath == "" {
		return virtualWorkspaceRoot
	}

	return virtualWorkspaceRoot + "/" + relativePath
}

func findSource(sources []outbound.WorkspaceSource, sourceID string) (outbound.WorkspaceSource, bool) {
	for _, source := range sources {
		if source.ID == sourceID {
			return source, true
		}
	}

	return outbound.WorkspaceSource{}, false
}

func findSources(
	sources []outbound.WorkspaceSource,
	sourceIDs []string,
) ([]outbound.WorkspaceSource, string) {
	found := make([]outbound.WorkspaceSource, 0, len(sourceIDs))
	for _, sourceID := range sourceIDs {
		source, ok := findSource(sources, sourceID)
		if !ok {
			return nil, fmt.Sprintf("workspace source is not available: %s", sourceID)
		}
		found = append(found, source)
	}

	return found, ""
}

func parseSourceIDs(rawValue string) []string {
	parts := strings.FieldsFunc(rawValue, func(value rune) bool {
		return value == ',' || value == '\n' || value == '\t' || value == ' '
	})
	if len(parts) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(parts))
	sourceIDs := make([]string, 0, len(parts))
	for _, part := range parts {
		sourceID := strings.TrimSpace(part)
		if sourceID == "" {
			continue
		}
		if _, ok := seen[sourceID]; ok {
			continue
		}
		seen[sourceID] = struct{}{}
		sourceIDs = append(sourceIDs, sourceID)
	}

	return sourceIDs
}

func findStagedPath(
	files []outbound.WorkspaceStagedFile,
	path string,
) (outbound.WorkspaceStagedFile, bool) {
	for _, file := range files {
		if file.Path == path {
			return file, true
		}
	}

	return outbound.WorkspaceStagedFile{}, false
}

func stagedFileIndex(files []outbound.WorkspaceStagedFile) map[string]outbound.WorkspaceStagedFile {
	if len(files) == 0 {
		return nil
	}
	index := make(map[string]outbound.WorkspaceStagedFile, len(files))
	for _, file := range files {
		index[file.Path] = file
	}

	return index
}

func stagedPathIndex(files []outbound.WorkspaceStagedFile) map[string][]string {
	index := make(map[string][]string)
	for _, file := range files {
		index[file.SourceID] = append(index[file.SourceID], virtualPath(file.Path))
	}
	for sourceID := range index {
		sort.Strings(index[sourceID])
	}

	return index
}

func safeWorkspaceError(err error) string {
	message := strings.TrimSpace(err.Error())
	switch {
	case message == "":
		return "workspace operation failed"
	case strings.Contains(message, "already exists"):
		return message
	case strings.Contains(message, "source is not available"):
		return message
	case strings.Contains(message, "unsafe workspace file path"):
		return messagePathUnsafe
	default:
		return "workspace operation failed"
	}
}
