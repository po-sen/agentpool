package agent

import (
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestBuildToolObservationFormatsSuccess(t *testing.T) {
	got := buildToolObservation("read_file", outbound.ToolResult{Content: "# Demo\n"})
	want := "Tool result for read_file:\n# Demo\n"
	if got != want {
		t.Fatalf("observation = %q, want %q", got, want)
	}
}

func TestBuildToolObservationFormatsError(t *testing.T) {
	got := buildToolObservation("read_file", outbound.ToolResult{Content: "file not found", IsError: true})
	want := "Tool error for read_file:\nfile not found"
	if got != want {
		t.Fatalf("observation = %q, want %q", got, want)
	}
}
