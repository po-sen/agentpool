package agent

import (
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestBuildToolObservationFormatsSuccess(t *testing.T) {
	got := buildToolObservation("workspace", outbound.ToolResult{Content: "files:\n/workspace/input/README.md\n"})
	want := "Tool result for workspace:\nfiles:\n/workspace/input/README.md\n"
	if got != want {
		t.Fatalf("observation = %q, want %q", got, want)
	}
}

func TestBuildToolObservationFormatsError(t *testing.T) {
	got := buildToolObservation("workspace", outbound.ToolResult{Content: "path is not available", IsError: true})
	want := "Tool error for workspace:\npath is not available"
	if got != want {
		t.Fatalf("observation = %q, want %q", got, want)
	}
}
