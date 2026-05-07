package agent

import (
	"strings"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

type strategyApproach string

const (
	strategyApproachUnknown          strategyApproach = "unknown"
	strategyApproachWorkspaceControl strategyApproach = "workspace_control"
	strategyApproachInlineCommand    strategyApproach = "inline_command"
	strategyApproachScriptCommand    strategyApproach = "script_command"
	strategyApproachCapabilityProbe  strategyApproach = "capability_probe"
)

const (
	strategyDirectiveHeader      = "Strategy directive:\n"
	requiredStrategyChangePrefix = "Required strategy change: "
)

func buildToolErrorStrategyDirective(tool string, arguments map[string]string, result outbound.ToolResult) string {
	if !result.IsError {
		return ""
	}

	if strings.TrimSpace(tool) != "sandbox_exec" {
		return genericStrategyDirective()
	}

	command := strings.TrimSpace(arguments["command"])
	resultText := strings.ToLower(result.Content)
	switch {
	case commandLooksLikeInlinePython(command) && strings.Contains(resultText, "syntaxerror"):
		return strategyDirectiveHeader +
			"- Previous approach: inline command failed with a syntax error.\n" +
			"- Next attempt must change approach: create a multi-line script file under /workspace, or use a newline-safe script invocation, then run it with sandbox_exec.\n" +
			"- If capability is uncertain, inspect installed commands or packages first.\n"
	case strings.Contains(resultText, "command not found") || strings.Contains(resultText, "not found"):
		return strategyDirectiveHeader +
			"- Previous approach used an unavailable command or path.\n" +
			"- Next attempt must inspect available commands, files, or packages, or switch to another installed command/library.\n"
	case strings.Contains(resultText, "no module named") || strings.Contains(resultText, "modulenotfounderror"):
		return strategyDirectiveHeader +
			"- Previous approach used an unavailable language package.\n" +
			"- Next attempt must inspect installed packages or switch to standard-library or installed alternatives.\n"
	case strings.Contains(resultText, "timed_out: true"):
		return strategyDirectiveHeader +
			"- Previous approach timed out.\n" +
			"- Next attempt must narrow the work, split it into smaller commands, or set a justified timeout_seconds value.\n"
	default:
		return genericStrategyDirective()
	}
}

func buildRepeatedToolCallStrategyDirective(tool string, arguments map[string]string) string {
	switch classifyToolApproach(tool, arguments) {
	case strategyApproachInlineCommand:
		return requiredStrategyChangePrefix + "replace the repeated inline command with a different approach, such as a script file, pipeline, capability probe, or a command with materially changed arguments."
	case strategyApproachWorkspaceControl:
		return requiredStrategyChangePrefix + "use the prior workspace observation; do not repeat the same control-plane operation. List current files, restore only if needed, or move to sandbox_exec."
	default:
		return requiredStrategyChangePrefix + "use the prior observation, change arguments, inspect capabilities, create a script or pipeline, or choose another available tool."
	}
}

func classifyToolApproach(tool string, arguments map[string]string) strategyApproach {
	switch strings.TrimSpace(tool) {
	case "workspace":
		return strategyApproachWorkspaceControl
	case "sandbox_exec":
		command := strings.TrimSpace(arguments["command"])
		switch {
		case commandLooksLikeCapabilityProbe(command):
			return strategyApproachCapabilityProbe
		case commandLooksLikeScriptCommand(command):
			return strategyApproachScriptCommand
		case command != "":
			return strategyApproachInlineCommand
		default:
			return strategyApproachUnknown
		}
	default:
		return strategyApproachUnknown
	}
}

func genericStrategyDirective() string {
	return strategyDirectiveHeader +
		"- Do not repeat the same tool call.\n" +
		"- Next attempt must change approach: inspect available files or capabilities, read existing artifacts, fix syntax or quoting with changed input, create a script or pipeline under /workspace, or use another installed command/library.\n"
}

func commandLooksLikeInlinePython(command string) bool {
	lower := strings.ToLower(command)

	return strings.Contains(lower, "python") && strings.Contains(lower, " -c ")
}

func commandLooksLikeScriptCommand(command string) bool {
	lower := strings.ToLower(command)
	if strings.Contains(command, "\n") || strings.Contains(lower, "<<") {
		return true
	}

	for _, marker := range []string{".py", ".sh", " tee ", " cat > ", "printf "} {
		if strings.Contains(lower, marker) {
			return true
		}
	}

	return false
}

func commandLooksLikeCapabilityProbe(command string) bool {
	lower := strings.ToLower(strings.TrimSpace(command))
	for _, prefix := range []string{
		"which ",
		"command -v ",
		"file ",
		"python3 -m pip",
		"python -m pip",
	} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}

	return false
}
