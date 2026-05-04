package workspace

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

const (
	listFilesToolName = "list_files"
	readFileToolName  = "read_file"
	defaultMaxBytes   = int64(65536)
	defaultMaxEntries = 200
	maxSymlinkDepth   = 32
)

// Config controls workspace tool limits.
type Config struct {
	MaxFileBytes   int64
	MaxListEntries int
}

// Runner executes read-only workspace tools.
type Runner struct {
	maxFileBytes   int64
	maxListEntries int
}

var _ outbound.ToolRunner = (*Runner)(nil)

// NewRunner creates a read-only workspace tool runner.
func NewRunner(cfg Config) *Runner {
	maxBytes := cfg.MaxFileBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}
	maxEntries := cfg.MaxListEntries
	if maxEntries <= 0 {
		maxEntries = defaultMaxEntries
	}

	return &Runner{
		maxFileBytes:   maxBytes,
		maxListEntries: maxEntries,
	}
}

// ListTools returns read-only workspace tool definitions.
func (r *Runner) ListTools(context.Context, outbound.ToolListRequest) ([]outbound.ToolDefinition, error) {
	return []outbound.ToolDefinition{
		{
			Name:        listFilesToolName,
			Description: "Lists files and directories under the run workspace.",
		},
		{
			Name:        readFileToolName,
			Description: "Reads a UTF-8 text file under the run workspace.",
		},
	}, nil
}

// RunTool executes a read-only workspace tool.
func (r *Runner) RunTool(_ context.Context, call outbound.ToolCall) (outbound.ToolResult, error) {
	switch call.Name {
	case listFilesToolName:
		return r.listFiles(call.Sandbox.WorkspacePath, call.Arguments["path"]), nil
	case readFileToolName:
		return r.readFile(call.Sandbox.WorkspacePath, call.Arguments["path"]), nil
	default:
		return toolError(fmt.Sprintf("unknown tool: %s", call.Name)), nil
	}
}

func (r *Runner) listFiles(root string, requestedPath string) outbound.ToolResult {
	if strings.TrimSpace(requestedPath) == "" {
		requestedPath = "."
	}

	dirPath, failure := resolveWorkspacePath(root, requestedPath)
	if failure != "" {
		return toolError(failure)
	}

	info, err := os.Stat(dirPath)
	if err != nil {
		return toolError("path not found")
	}
	if !info.IsDir() {
		return toolError("path is not a directory")
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return toolError("cannot list directory")
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		names = append(names, name)
	}
	sort.Strings(names)

	truncated := false
	if len(names) > r.maxListEntries {
		names = names[:r.maxListEntries]
		truncated = true
	}
	if truncated {
		names = append(names, fmt.Sprintf("... truncated after %d entries", r.maxListEntries))
	}

	return outbound.ToolResult{Content: strings.Join(names, "\n")}
}

func (r *Runner) readFile(root string, requestedPath string) outbound.ToolResult {
	if strings.TrimSpace(requestedPath) == "" {
		return toolError("missing path argument")
	}

	filePath, failure := resolveWorkspacePath(root, requestedPath)
	if failure != "" {
		return toolError(failure)
	}

	info, err := os.Stat(filePath)
	if err != nil {
		return toolError("path not found")
	}
	if info.IsDir() {
		return toolError("path is a directory")
	}
	if info.Size() > r.maxFileBytes {
		return toolError(fmt.Sprintf("file exceeds %d bytes", r.maxFileBytes))
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return toolError("cannot read file")
	}
	if int64(len(content)) > r.maxFileBytes {
		return toolError(fmt.Sprintf("file exceeds %d bytes", r.maxFileBytes))
	}
	if bytes.IndexByte(content, 0) >= 0 {
		return toolError("file is binary")
	}
	if !utf8.Valid(content) {
		return toolError("file is not UTF-8 text")
	}

	return outbound.ToolResult{Content: string(content)}
}

func resolveWorkspacePath(root string, requestedPath string) (string, string) {
	if strings.TrimSpace(root) == "" {
		return "", "workspace path is not available"
	}
	if filepath.IsAbs(requestedPath) {
		return "", "path must be relative"
	}

	workspaceRoot, err := filepath.Abs(root)
	if err != nil {
		return "", "workspace path is not available"
	}
	workspaceRoot, err = filepath.EvalSymlinks(workspaceRoot)
	if err != nil {
		return "", "workspace path is not available"
	}

	cleanedPath := filepath.Clean(filepath.FromSlash(requestedPath))
	if cleanedPath == ".." || strings.HasPrefix(cleanedPath, ".."+string(os.PathSeparator)) {
		return "", "path escapes workspace"
	}

	return resolveRelativePath(workspaceRoot, cleanedPath, 0)
}

func resolveRelativePath(root string, relativePath string, depth int) (string, string) {
	currentPath := root
	for _, part := range pathParts(relativePath) {
		resolvedPath, failure := resolveExistingPath(root, filepath.Join(currentPath, part), depth)
		if failure != "" {
			return "", failure
		}
		currentPath = resolvedPath
	}

	return currentPath, ""
}

func resolveExistingPath(root string, targetPath string, depth int) (string, string) {
	if !isWithinRoot(root, targetPath) {
		return "", "path escapes workspace"
	}

	info, err := os.Lstat(targetPath)
	if err != nil {
		return "", "path not found"
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return targetPath, ""
	}

	return resolveSymlink(root, targetPath, depth+1)
}

func resolveSymlink(root string, linkPath string, depth int) (string, string) {
	if depth > maxSymlinkDepth {
		return "", "path not found"
	}

	target, err := os.Readlink(linkPath)
	if err != nil {
		return "", "path not found"
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(linkPath), target)
	}

	targetPath, err := filepath.Abs(target)
	if err != nil {
		return "", "path not found"
	}
	targetPath = filepath.Clean(targetPath)
	if !isWithinRoot(root, targetPath) {
		return "", "path escapes workspace"
	}

	relativePath, err := filepath.Rel(root, targetPath)
	if err != nil {
		return "", "path escapes workspace"
	}

	return resolveRelativePath(root, relativePath, depth)
}

func pathParts(relativePath string) []string {
	if relativePath == "." {
		return nil
	}

	return strings.Split(relativePath, string(os.PathSeparator))
}

func isWithinRoot(root string, target string) bool {
	relativePath, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}

	return relativePath == "." ||
		(relativePath != ".." && !strings.HasPrefix(relativePath, ".."+string(os.PathSeparator)))
}

func toolError(content string) outbound.ToolResult {
	return outbound.ToolResult{
		Content: content,
		IsError: true,
	}
}
