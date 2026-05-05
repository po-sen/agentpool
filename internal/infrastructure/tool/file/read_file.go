package file

import (
	"os"
	"unicode/utf8"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

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
