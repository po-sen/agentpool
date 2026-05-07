package agent

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestBuildToolObservationFormatsSuccess(t *testing.T) {
	got := buildToolObservation("workspace", nil, outbound.ToolResult{Content: "files:\n/workspace/README.md\n"})
	want := "Tool result for workspace:\nfiles:\n/workspace/README.md\n"
	if got != want {
		t.Fatalf("observation = %q, want %q", got, want)
	}
}

func TestBuildToolObservationFormatsError(t *testing.T) {
	got := buildToolObservation("workspace", nil, outbound.ToolResult{Content: "path is not available", IsError: true})
	for _, want := range []string{
		"Tool error for workspace:",
		"This failed attempt is an observation, not a final answer.",
		"Do not repeat the same tool call",
		"keep iterating with changed arguments or a different method until a tool call succeeds.",
		"Strategy directive:",
		"path is not available",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("observation = %q, want substring %q", got, want)
		}
	}
}

func TestBuildToolObservationIncludesStrategyDirectiveForInlineSyntaxError(t *testing.T) {
	got := buildToolObservation("sandbox_exec", map[string]string{
		"command": `python3 -c "if True: print(42); else: print(0)"`,
	}, outbound.ToolResult{
		Content: "exit_code: 1\nstderr:\nSyntaxError: invalid syntax\n",
		IsError: true,
	})

	for _, want := range []string{
		"Previous approach: inline command failed with a syntax error.",
		"create a multi-line script file under /workspace",
		"SyntaxError: invalid syntax",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("observation = %q, want substring %q", got, want)
		}
	}
}

func TestBuildToolObservationDoesNotAddTaskSpecificHints(t *testing.T) {
	got := buildToolObservation("sandbox_exec", map[string]string{
		"command": "some domain-specific command",
	}, outbound.ToolResult{
		Content: "exit_code: 0\nstdout:\nraw output\n",
	})

	if strings.Contains(got, "Hint:") {
		t.Fatalf("observation added task-specific hint:\n%s", got)
	}
	if !strings.Contains(got, "Tool result for sandbox_exec:\nexit_code: 0\nstdout:\nraw output\n") {
		t.Fatalf("observation = %q, want raw tool result", got)
	}
}

func TestBuildToolObservationTruncatesLongContent(t *testing.T) {
	got := buildToolObservation("sandbox_exec", nil, outbound.ToolResult{
		Content: strings.Repeat("界", maxToolObservationLength),
		IsError: true,
	})

	if len(got) > maxToolObservationLength {
		t.Fatalf("len(observation) = %d, want <= %d", len(got), maxToolObservationLength)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("observation is not valid UTF-8: %q", got)
	}
	if !strings.HasSuffix(got, agentTurnPreviewTruncatedMarker) {
		t.Fatalf("observation does not contain truncation marker: %q", got)
	}
}
