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
	argumentArea      = "area"
	argumentPath      = "path"

	operationList = "list"
	operationStat = "stat"

	areaInput = "input"
	areaWork  = "work"
	areaAll   = "all"

	defaultMaxFiles = 200

	virtualWorkspaceRoot = "/workspace"
	messagePathUnsafe    = "path is unsafe"
)

var errTooManyFiles = errors.New("too many files")

// Config controls workspace metadata tool limits.
type Config struct {
	MaxFiles int
}

// Runner exposes workspace metadata operations.
type Runner struct {
	maxFiles int
}

var _ outbound.ToolRunner = (*Runner)(nil)

// NewRunner creates a workspace metadata tool runner.
func NewRunner(cfg Config) *Runner {
	maxFiles := cfg.MaxFiles
	if maxFiles <= 0 {
		maxFiles = defaultMaxFiles
	}

	return &Runner{maxFiles: maxFiles}
}

// ListTools exposes workspace when structured workspace paths are available.
func (r *Runner) ListTools(_ context.Context, request outbound.ToolListRequest) ([]outbound.ToolDefinition, error) {
	if !workspaceAvailable(request.Context.Workspace) {
		return nil, nil
	}

	return []outbound.ToolDefinition{
		{
			Name:        toolNameWorkspace,
			Description: "Lists or stats workspace paths without reading file contents.",
			Arguments: []outbound.ToolArgumentDefinition{
				{
					Name:        argumentOperation,
					Description: `Operation to run. Supported values: "list" or "stat".`,
					Required:    true,
					Example:     "list",
				},
				{
					Name:        argumentArea,
					Description: `Workspace area to inspect. Supported values: "input", "work", or "all". Defaults to "all".`,
					Required:    false,
					Example:     "input",
				},
				{
					Name:        argumentPath,
					Description: `Safe relative path inside the selected area, or a /workspace/input|work virtual path. Defaults to ".".`,
					Required:    false,
					Example:     "/workspace/input/README.md",
				},
			},
		},
	}, nil
}

// RunTool executes a workspace metadata operation.
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
	areas, relativePath, selectionMessage := resolveSelection(
		call.Context.Workspace,
		call.Arguments[argumentArea],
		call.Arguments[argumentPath],
	)
	if selectionMessage != "" {
		return outbound.ToolResult{Content: selectionMessage, IsError: true}, nil
	}

	switch operation {
	case operationList:
		return r.list(ctx, areas, relativePath), nil
	case operationStat:
		return r.stat(areas, relativePath), nil
	default:
		return outbound.ToolResult{Content: fmt.Sprintf("unknown workspace operation: %s", operation), IsError: true}, nil
	}
}

func workspaceAvailable(workspace outbound.Workspace) bool {
	return strings.TrimSpace(workspace.InputPath) != "" && strings.TrimSpace(workspace.WorkPath) != ""
}

type workspaceArea struct {
	name string
	root string
}

func selectedAreas(workspace outbound.Workspace, rawArea string) ([]workspaceArea, string) {
	area := strings.TrimSpace(rawArea)
	if area == "" {
		area = areaAll
	}

	switch area {
	case areaInput:
		return []workspaceArea{{name: areaInput, root: workspace.InputPath}}, ""
	case areaWork:
		return []workspaceArea{{name: areaWork, root: workspace.WorkPath}}, ""
	case areaAll:
		return []workspaceArea{
			{name: areaInput, root: workspace.InputPath},
			{name: areaWork, root: workspace.WorkPath},
		}, ""
	default:
		return nil, fmt.Sprintf("unknown workspace area: %s", area)
	}
}

func resolveSelection(workspace outbound.Workspace, rawArea string, rawPath string) ([]workspaceArea, string, string) {
	if area, relativePath, ok, message := virtualPathSelection(rawPath); ok || message != "" {
		if message != "" {
			return nil, "", message
		}
		if conflict := virtualAreaConflict(rawArea, area); conflict != "" {
			return nil, "", conflict
		}
		areas, areaMessage := selectedAreas(workspace, area)
		if areaMessage != "" {
			return nil, "", areaMessage
		}

		return areas, relativePath, ""
	}

	areas, areaMessage := selectedAreas(workspace, rawArea)
	if areaMessage != "" {
		return nil, "", areaMessage
	}
	relativePath, pathMessage := safeRelativePath(rawPath)
	if pathMessage != "" {
		return nil, "", pathMessage
	}

	return areas, relativePath, ""
}

func virtualPathSelection(rawPath string) (string, string, bool, string) {
	cleanPath := strings.TrimSpace(rawPath)
	if cleanPath == "" {
		return "", "", false, ""
	}
	if strings.HasPrefix(cleanPath, virtualWorkspaceRoot+"/") || cleanPath == virtualWorkspaceRoot {
		area, relativePath, ok := splitVirtualPath(cleanPath)
		if !ok {
			return "", "", false, messagePathUnsafe
		}
		if relativePath == "" {
			relativePath = "."
		}
		if message := validateRelativePath(relativePath); message != "" {
			return "", "", false, message
		}

		return area, relativePath, true, ""
	}
	if path.IsAbs(cleanPath) || filepath.IsAbs(cleanPath) {
		return "", "", false, messagePathUnsafe
	}

	return "", "", false, ""
}

func splitVirtualPath(cleanPath string) (string, string, bool) {
	for _, area := range []string{areaInput, areaWork} {
		prefix := virtualWorkspaceRoot + "/" + area
		switch {
		case cleanPath == prefix:
			return area, ".", true
		case strings.HasPrefix(cleanPath, prefix+"/"):
			return area, strings.TrimPrefix(cleanPath, prefix+"/"), true
		}
	}

	return "", "", false
}

func virtualAreaConflict(rawArea string, virtualArea string) string {
	area := strings.TrimSpace(rawArea)
	if area == "" || area == areaAll || area == virtualArea {
		return ""
	}

	return fmt.Sprintf("workspace area %q conflicts with virtual path area %q", area, virtualArea)
}

func safeRelativePath(rawPath string) (string, string) {
	cleanPath := strings.TrimSpace(rawPath)
	if cleanPath == "" {
		return ".", ""
	}
	if message := validateRelativePath(cleanPath); message != "" {
		return "", message
	}

	return cleanPath, ""
}

func validateRelativePath(cleanPath string) string {
	if path.IsAbs(cleanPath) ||
		filepath.IsAbs(cleanPath) ||
		strings.Contains(cleanPath, "\\") ||
		strings.Contains(cleanPath, ":") ||
		path.Clean(cleanPath) != cleanPath {
		return messagePathUnsafe
	}
	if cleanPath == "." {
		return ""
	}
	for _, component := range strings.Split(cleanPath, "/") {
		if component == "" || component == "." || component == ".." {
			return messagePathUnsafe
		}
	}

	return ""
}

func confinedPath(area workspaceArea, relativePath string) (string, error) {
	target := area.root
	if relativePath != "." {
		target = filepath.Join(area.root, filepath.FromSlash(relativePath))
	}
	relative, err := filepath.Rel(area.root, target)
	if err != nil ||
		relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) ||
		filepath.IsAbs(relative) {
		return "", errors.New("path escapes workspace")
	}

	return target, nil
}

func (r *Runner) list(ctx context.Context, areas []workspaceArea, relativePath string) outbound.ToolResult {
	var files []workspaceFile
	foundPath := false
	for _, area := range areas {
		areaFiles, found, err := r.collectFiles(ctx, area, relativePath, len(files))
		if errors.Is(err, errTooManyFiles) {
			return outbound.ToolResult{Content: "workspace contains too many files to list", IsError: true}
		}
		if err != nil {
			return outbound.ToolResult{Content: "list workspace failed", IsError: true}
		}
		foundPath = foundPath || found
		files = append(files, areaFiles...)
	}

	if !foundPath {
		return outbound.ToolResult{Content: "path is not available", IsError: true}
	}
	sort.Slice(files, func(i int, j int) bool {
		return files[i].virtualPath < files[j].virtualPath
	})
	if len(files) == 0 {
		return outbound.ToolResult{Content: "files:\n"}
	}

	return outbound.ToolResult{Content: "files:\n" + formatWorkspaceFiles(files)}
}

type workspaceFile struct {
	virtualPath  string
	area         string
	relativePath string
	sizeBytes    int64
}

func (r *Runner) collectFiles(
	ctx context.Context,
	area workspaceArea,
	relativePath string,
	currentCount int,
) ([]workspaceFile, bool, error) {
	root, err := confinedPath(area, relativePath)
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
		file, ok, err := collectWorkspaceFile(ctx, area, current, entry, err)
		if err != nil || !ok {
			return err
		}
		files = append(files, file)
		if currentCount+len(files) > r.maxFiles {
			return errTooManyFiles
		}

		return nil
	})

	return files, true, err
}

func collectWorkspaceFile(
	ctx context.Context,
	area workspaceArea,
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

	relative, err := filepath.Rel(area.root, current)
	if err != nil {
		return workspaceFile{}, false, err
	}
	relativePath := filepath.ToSlash(relative)

	return workspaceFile{
		virtualPath:  virtualPath(area.name, relativePath),
		area:         area.name,
		relativePath: relativePath,
		sizeBytes:    info.Size(),
	}, true, nil
}

func formatWorkspaceFiles(files []workspaceFile) string {
	var builder strings.Builder
	for _, file := range files {
		builder.WriteString("- virtual_path: ")
		builder.WriteString(file.virtualPath)
		builder.WriteString("\n  area: ")
		builder.WriteString(file.area)
		builder.WriteString("\n  relative_path: ")
		builder.WriteString(file.relativePath)
		builder.WriteString("\n  size_bytes: ")
		_, _ = fmt.Fprintf(&builder, "%d", file.sizeBytes)
		builder.WriteString("\n")
	}

	return builder.String()
}

func (r *Runner) stat(areas []workspaceArea, relativePath string) outbound.ToolResult {
	lines := make([]string, 0, len(areas)*5)
	for _, area := range areas {
		target, err := confinedPath(area, relativePath)
		if err != nil {
			return outbound.ToolResult{Content: err.Error(), IsError: true}
		}
		info, err := os.Lstat(target)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}

			return outbound.ToolResult{Content: "stat workspace path failed", IsError: true}
		}

		lines = append(lines, formatStat(area, relativePath, info)...)
	}
	if len(lines) == 0 {
		return outbound.ToolResult{Content: "path is not available", IsError: true}
	}

	return outbound.ToolResult{Content: strings.Join(lines, "\n") + "\n"}
}

func formatStat(area workspaceArea, relativePath string, info fs.FileInfo) []string {
	kind := "other"
	switch {
	case info.IsDir():
		kind = "directory"
	case info.Mode().IsRegular():
		kind = "file"
	}

	return []string{
		"virtual_path: " + virtualPath(area.name, relativePath),
		"area: " + area.name,
		"relative_path: " + relativePath,
		fmt.Sprintf("size_bytes: %d", info.Size()),
		"type: " + kind,
	}
}

func virtualPath(area string, relativePath string) string {
	if relativePath == "." || relativePath == "" {
		return virtualWorkspaceRoot + "/" + area
	}

	return virtualWorkspaceRoot + "/" + area + "/" + relativePath
}
