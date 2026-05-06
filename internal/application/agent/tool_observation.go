package agent

import (
	"fmt"
	"strings"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

const maxToolObservationLength = 8 << 10

func buildToolObservation(tool string, arguments map[string]string, result outbound.ToolResult) string {
	content := result.Content
	if sandboxExecPDFSearchFailed(tool, arguments, result) {
		content = strings.TrimRight(content, "\n") + `
Hint: This PDF search produced no usable stdout. Do not repeat the exact same grep. Search broader relevant terms across all PDFs, for example:
Choose concise task-specific terms, not only exact prompt nouns; avoid broad one-character terms such as water/fire alone.
After finding relevant lines, inspect nearby context before final; do not only list hits.
for f in /workspace/input/*.pdf; do printf 'FILE:%s\n' "$f"; pdftotext "$f" - 2>/dev/null | grep -nEi 'term1|term2|term3' | head -20; done
`
	}
	if sandboxExecPDFSearchSucceeded(tool, arguments, result) {
		content = "Hint: These are search hits only. Inspect nearby context around relevant lines before final, synthesize the answer first, and cite file names/locations.\n" + content
	}
	if result.IsError {
		prefix := fmt.Sprintf("Tool error for %s:\n", tool)

		return prefix + truncateToolObservationContent(prefix, content)
	}

	prefix := fmt.Sprintf("Tool result for %s:\n", tool)

	return prefix + truncateToolObservationContent(prefix, content)
}

func sandboxExecPDFSearchFailed(tool string, arguments map[string]string, result outbound.ToolResult) bool {
	if !result.IsError || strings.TrimSpace(tool) != toolNameSandboxExec {
		return false
	}
	command := strings.ToLower(arguments[toolArgumentCommand])

	return strings.Contains(command, "pdftotext") && strings.Contains(command, "grep")
}

func sandboxExecPDFSearchSucceeded(tool string, arguments map[string]string, result outbound.ToolResult) bool {
	if result.IsError || strings.TrimSpace(tool) != toolNameSandboxExec {
		return false
	}
	if !strings.Contains(result.Content, "FILE:/workspace/input/") {
		return false
	}
	command := strings.ToLower(arguments[toolArgumentCommand])

	return strings.Contains(command, "pdftotext") && strings.Contains(command, "grep")
}

func truncateToolObservationContent(prefix string, content string) string {
	budget := maxToolObservationLength - len(prefix)
	if budget <= 0 {
		return ""
	}

	return truncateAgentTurnText(content, budget)
}
