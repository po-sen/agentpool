package shell

import (
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestFormatCommandResultIncludesExitCodeAndOutputBlocks(t *testing.T) {
	result := formatCommandResult(outbound.SandboxCommandResult{
		ExitCode: 2,
		Stdout:   "hello",
		Stderr:   "failed\n",
	})

	want := "exit_code: 2\nstdout:\nhello\nstderr:\nfailed\n"
	if result != want {
		t.Fatalf("formatCommandResult() = %q, want %q", result, want)
	}
}

func TestFormatCommandResultIncludesTimedOutWhenTrue(t *testing.T) {
	result := formatCommandResult(outbound.SandboxCommandResult{
		ExitCode: 124,
		TimedOut: true,
	})

	want := "exit_code: 124\ntimed_out: true\nstdout:\n\nstderr:\n\n"
	if result != want {
		t.Fatalf("formatCommandResult() = %q, want %q", result, want)
	}
}
