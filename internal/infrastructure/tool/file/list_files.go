package file

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

var errTooManyFiles = errors.New("too many files")

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
