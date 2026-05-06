package agent

import (
	"fmt"
	"strings"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

const (
	maxToolObservationLength = 8 << 10
	pdfToTextCommandName     = "pdftotext"
	pdfSearchScriptName      = "pdf_search"
	pdfContextScriptName     = "pdf_context"
)

func buildToolObservation(tool string, arguments map[string]string, result outbound.ToolResult) string {
	content := result.Content
	if sandboxExecPDFSearchFailed(tool, arguments, result) {
		content = strings.TrimRight(content, "\n") + `
Hint: This PDF search produced no usable stdout. Do not repeat the exact same grep. Search broader relevant terms across all PDFs, for example:
Choose concise task-specific terms, not only exact prompt nouns. Avoid broad one-character terms and file-title/TOC terms.
Use pdftotext with "-" for stdout. If this is getting complex, write your own small script under /workspace/work, then run it.
After finding relevant lines, inspect nearby context before final; do not only list hits.
for f in /workspace/input/*.pdf; do printf 'FILE:%s\n' "$f"; pdftotext "$f" - 2>/dev/null | grep -nEi 'term1|term2|term3' | head -20; done
`
	}
	if sandboxExecPDFSearchSucceeded(tool, arguments, result) {
		content = "Hint: These are search hits only, not the answer. Ignore title/TOC-only hits; if hits are not substantive, search again with task-specific terms. Next inspect nearby context around substantive line numbers with a different command, for example: pdftotext '/workspace/input/manual.pdf' - 2>/dev/null | nl -ba | sed -n '512,528p'. Final answer must use the user's language, answer first, and cite file names/locations; do not claim a procedure exists unless context says it.\n" + content
	}
	if sandboxExecPDFContextSucceeded(tool, arguments, result) {
		content = "Hint: This is nearby PDF context. If it is title/TOC-only or irrelevant, say the provided files do not contain the requested instructions based on the searches/context inspected. Answer in the user's language (中文問題請用中文回答), preserve the user's exact terms, cite the file name and inspected location, and do not tell the user to search the document.\n" + content
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

	return commandIsPDFSearch(command)
}

func sandboxExecPDFSearchSucceeded(tool string, arguments map[string]string, result outbound.ToolResult) bool {
	if result.IsError || strings.TrimSpace(tool) != toolNameSandboxExec {
		return false
	}
	if sandboxExecResultStdout(result.Content) == "" {
		return false
	}
	command := strings.ToLower(arguments[toolArgumentCommand])

	return commandIsPDFSearch(command)
}

func sandboxExecPDFContextSucceeded(tool string, arguments map[string]string, result outbound.ToolResult) bool {
	if result.IsError || strings.TrimSpace(tool) != toolNameSandboxExec {
		return false
	}
	if sandboxExecResultStdout(result.Content) == "" {
		return false
	}
	command := strings.ToLower(arguments[toolArgumentCommand])

	return commandIsPDFContext(command)
}

func commandIsPDFSearch(command string) bool {
	return (strings.Contains(command, pdfToTextCommandName) && strings.Contains(command, "grep")) ||
		strings.Contains(command, pdfSearchScriptName)
}

func commandIsPDFContext(command string) bool {
	return (strings.Contains(command, pdfToTextCommandName) &&
		!strings.Contains(command, "grep") &&
		(strings.Contains(command, "sed") || strings.Contains(command, "awk") || strings.Contains(command, "nl"))) ||
		strings.Contains(command, pdfContextScriptName)
}

func sandboxExecResultStdout(content string) string {
	const stdoutPrefix = "stdout:\n"
	stdoutIndex := strings.Index(content, stdoutPrefix)
	if stdoutIndex < 0 {
		return strings.TrimSpace(content)
	}

	stdout := content[stdoutIndex+len(stdoutPrefix):]
	if stderrIndex := strings.Index(stdout, "\nstderr:"); stderrIndex >= 0 {
		stdout = stdout[:stderrIndex]
	}

	return strings.TrimSpace(stdout)
}

func truncateToolObservationContent(prefix string, content string) string {
	budget := maxToolObservationLength - len(prefix)
	if budget <= 0 {
		return ""
	}

	return truncateAgentTurnText(content, budget)
}
