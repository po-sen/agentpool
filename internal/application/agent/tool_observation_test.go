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
		"This may be partial or expected",
		"try a different approach before final.",
		"path is not available",
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
