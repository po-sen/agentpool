package file

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
	"unicode/utf8"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

const (
	toolNameListFiles = "list_files"
	toolNameReadFile  = "read_file"

	argumentPath = "path"

	defaultMaxFiles         = 200
	defaultMaxReadSizeBytes = 256 << 10
)

// Config controls read-only workspace file tool limits.
type Config struct {
	MaxFiles         int
	MaxReadSizeBytes int64
}

// Runner exposes read-only workspace file tools.
type Runner struct {
	maxFiles         int
	maxReadSizeBytes int64
}

var _ outbound.ToolRunner = (*Runner)(nil)

// NewRunner creates a read-only workspace file tool runner.
func NewRunner(cfg Config) *Runner {
	maxFiles := cfg.MaxFiles
	if maxFiles <= 0 {
		maxFiles = defaultMaxFiles
	}
	maxReadSizeBytes := cfg.MaxReadSizeBytes
	if maxReadSizeBytes <= 0 {
		maxReadSizeBytes = defaultMaxReadSizeBytes
	}

	return &Runner{
		maxFiles:         maxFiles,
		maxReadSizeBytes: maxReadSizeBytes,
	}
}

// ListTools exposes file tools only when a workspace path is available.
func (r *Runner) ListTools(_ context.Context, request outbound.ToolListRequest) ([]outbound.ToolDefinition, error) {
	if request.Context.WorkspacePath == "" {
		return nil, nil
	}

	return []outbound.ToolDefinition{
		{
			Name:        toolNameListFiles,
			Description: "Lists uploaded workspace files as relative paths.",
		},
		{
			Name:        toolNameReadFile,
			Description: "Reads one UTF-8 text file from the uploaded workspace. Argument: path.",
		},
	}, nil
}

// RunTool executes a read-only workspace file tool call.
func (r *Runner) RunTool(ctx context.Context, call outbound.ToolCall) (outbound.ToolResult, error) {
	if call.Context.WorkspacePath == "" {
		return outbound.ToolResult{Content: "workspace path is not available", IsError: true}, nil
	}

	switch call.Name {
	case toolNameListFiles:
		return r.listFiles(ctx, call.Context.WorkspacePath), nil
	case toolNameReadFile:
		return r.readFile(call.Context.WorkspacePath, call.Arguments[argumentPath]), nil
	default:
		return outbound.ToolResult{Content: fmt.Sprintf("unknown tool: %s", call.Name), IsError: true}, nil
	}
}

func (r *Runner) listFiles(ctx context.Context, root string) outbound.ToolResult {
	files, err := r.collectFiles(ctx, root)
	if errors.Is(err, errTooManyFiles) {
		return outbound.ToolResult{Content: "workspace contains too many files to list", IsError: true}
	}
	if err != nil {
		return outbound.ToolResult{Content: "list files failed", IsError: true}
	}

	sort.Strings(files)
	if len(files) == 0 {
		return outbound.ToolResult{Content: "files:\n"}
	}

	return outbound.ToolResult{Content: "files:\n" + strings.Join(files, "\n")}
}

func (r *Runner) collectFiles(ctx context.Context, root string) ([]string, error) {
	files := make([]string, 0)
	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if entry.IsDir() {
			return nil
		}

		relative, ok, err := regularRelativePath(root, current, entry)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}

		files = append(files, relative)
		if len(files) > r.maxFiles {
			return errTooManyFiles
		}

		return nil
	})

	return files, err
}

func regularRelativePath(root string, current string, entry fs.DirEntry) (string, bool, error) {
	info, err := entry.Info()
	if err != nil {
		return "", false, err
	}
	if !info.Mode().IsRegular() {
		return "", false, nil
	}

	relative, err := filepath.Rel(root, current)
	if err != nil {
		return "", false, err
	}

	return filepath.ToSlash(relative), true, nil
}

func (r *Runner) readFile(root string, requestedPath string) outbound.ToolResult {
	target, err := confinedPath(root, requestedPath)
	if err != nil {
		return outbound.ToolResult{Content: err.Error(), IsError: true}
	}

	info, err := os.Lstat(target)
	if err != nil {
		return outbound.ToolResult{Content: "file is not available", IsError: true}
	}
	if !info.Mode().IsRegular() {
		return outbound.ToolResult{Content: "path is not a regular file", IsError: true}
	}
	if info.Size() > r.maxReadSizeBytes {
		return outbound.ToolResult{Content: "file exceeds maximum read size", IsError: true}
	}

	content, err := os.ReadFile(target)
	if err != nil {
		return outbound.ToolResult{Content: "read file failed", IsError: true}
	}
	if !utf8.Valid(content) {
		return outbound.ToolResult{Content: "file is not UTF-8 text", IsError: true}
	}

	return outbound.ToolResult{Content: string(content)}
}

func confinedPath(root string, requestedPath string) (string, error) {
	cleanPath := strings.TrimSpace(requestedPath)
	if cleanPath == "" {
		return "", errors.New("path argument is required")
	}
	if path.IsAbs(cleanPath) ||
		filepath.IsAbs(cleanPath) ||
		strings.Contains(cleanPath, "\\") ||
		strings.Contains(cleanPath, ":") ||
		path.Clean(cleanPath) != cleanPath {
		return "", errors.New("path is unsafe")
	}
	for _, component := range strings.Split(cleanPath, "/") {
		if component == "" || component == "." || component == ".." {
			return "", errors.New("path is unsafe")
		}
	}

	target := filepath.Join(root, filepath.FromSlash(cleanPath))
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", errors.New("path escapes workspace")
	}

	return target, nil
}

var errTooManyFiles = errors.New("too many files")
