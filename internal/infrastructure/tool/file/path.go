package file

import (
	"errors"
	"path"
	"path/filepath"
	"strings"
)

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
