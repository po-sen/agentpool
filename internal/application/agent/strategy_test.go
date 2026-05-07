package agent

import (
	"strings"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestBuildToolErrorStrategyDirectiveRequiresScriptAfterInlinePythonSyntaxError(t *testing.T) {
	message := buildToolErrorStrategyDirective("sandbox_exec", map[string]string{
		"command": `python3 -c "if True: print(42); else: print(0)"`,
	}, outbound.ToolResult{
		Content: "exit_code: 1\nstderr:\nSyntaxError: invalid syntax\n",
		IsError: true,
	})

	for _, want := range []string{
		"Strategy directive:",
		"Previous approach: inline command failed with a syntax error.",
		"create a multi-line script file under /workspace",
		"newline-safe script invocation",
		"inspect installed commands or packages first",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("message does not contain %q:\n%s", want, message)
		}
	}
}

func TestBuildToolErrorStrategyDirectiveHandlesMissingCommand(t *testing.T) {
	message := buildToolErrorStrategyDirective("sandbox_exec", map[string]string{
		"command": "missing-tool --version",
	}, outbound.ToolResult{
		Content: "exit_code: 127\nstderr:\nmissing-tool: command not found\n",
		IsError: true,
	})

	for _, want := range []string{
		"Previous approach used an unavailable command or path.",
		"inspect available commands, files, or packages",
		"switch to another installed command/library",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("message does not contain %q:\n%s", want, message)
		}
	}
}

func TestBuildToolErrorStrategyDirectiveHandlesMissingPackage(t *testing.T) {
	message := buildToolErrorStrategyDirective("sandbox_exec", map[string]string{
		"command": `python3 -c "import missing_package"`,
	}, outbound.ToolResult{
		Content: "exit_code: 1\nstderr:\nModuleNotFoundError: No module named 'missing_package'\n",
		IsError: true,
	})

	for _, want := range []string{
		"Previous approach used an unavailable language package.",
		"inspect installed packages",
		"standard-library or installed alternatives",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("message does not contain %q:\n%s", want, message)
		}
	}
}

func TestBuildToolErrorStrategyDirectiveIsEmptyForSuccess(t *testing.T) {
	message := buildToolErrorStrategyDirective("sandbox_exec", nil, outbound.ToolResult{
		Content: "exit_code: 0\nstdout:\nok\n",
	})
	if message != "" {
		t.Fatalf("message = %q, want empty directive for success", message)
	}
}

func TestBuildRepeatedToolCallStrategyDirectiveForInlineCommand(t *testing.T) {
	message := buildRepeatedToolCallStrategyDirective("sandbox_exec", map[string]string{
		"command": `python3 -c "print(42)"`,
	})

	for _, want := range []string{
		"Required strategy change:",
		"replace the repeated inline command",
		"script file",
		"capability probe",
		"materially changed arguments",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("message does not contain %q:\n%s", want, message)
		}
	}
}

func TestBuildRepeatedToolCallStrategyDirectiveForWorkspaceControl(t *testing.T) {
	message := buildRepeatedToolCallStrategyDirective("workspace", map[string]string{
		"operation": "stage",
		"source_id": "input_001",
	})

	for _, want := range []string{
		"Required strategy change:",
		"use the prior workspace observation",
		"do not repeat the same control-plane operation",
		"move to sandbox_exec",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("message does not contain %q:\n%s", want, message)
		}
	}
}

func TestClassifyToolApproach(t *testing.T) {
	tests := []struct {
		name      string
		tool      string
		arguments map[string]string
		want      strategyApproach
	}{
		{
			name:      "workspace",
			tool:      "workspace",
			arguments: map[string]string{"operation": "list"},
			want:      strategyApproachWorkspaceControl,
		},
		{
			name:      "inline command",
			tool:      "sandbox_exec",
			arguments: map[string]string{"command": `python3 -c "print(42)"`},
			want:      strategyApproachInlineCommand,
		},
		{
			name:      "script command",
			tool:      "sandbox_exec",
			arguments: map[string]string{"command": "python3 /workspace/check.py"},
			want:      strategyApproachScriptCommand,
		},
		{
			name:      "capability probe",
			tool:      "sandbox_exec",
			arguments: map[string]string{"command": "which tesseract"},
			want:      strategyApproachCapabilityProbe,
		},
		{
			name:      "unknown",
			tool:      "unknown",
			arguments: nil,
			want:      strategyApproachUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyToolApproach(tt.tool, tt.arguments); got != tt.want {
				t.Fatalf("classifyToolApproach() = %q, want %q", got, tt.want)
			}
		})
	}
}
