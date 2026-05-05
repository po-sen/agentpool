package cli

import (
	"errors"
	"testing"
	"time"
)

func TestParseValidRuntimeCommands(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want CommandKind
	}{
		{name: "server", arg: commandServerName, want: CommandServer},
		{name: "worker", arg: commandWorkerName, want: CommandWorker},
		{name: "dev", arg: commandDevName, want: CommandDev},
		{name: "version", arg: commandVersionName, want: CommandVersion},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse([]string{tt.arg})
			if err != nil {
				t.Fatalf("parse command: %v", err)
			}
			if got.Kind != tt.want {
				t.Fatalf("command = %q, want %q", got.Kind, tt.want)
			}
		})
	}
}

func TestParseRunCommand(t *testing.T) {
	got, err := Parse([]string{
		commandRunName,
		"--prompt",
		"inspect",
		"--file",
		"README.md",
		"--file",
		"internal/application/workflow/worker.go",
		"--timeout",
		"3s",
		"--poll-interval",
		"250ms",
		"--debug",
	})
	if err != nil {
		t.Fatalf("parse run command: %v", err)
	}

	if got.Kind != CommandRun {
		t.Fatalf("command = %q, want %q", got.Kind, CommandRun)
	}
	if got.Run.Prompt != "inspect" {
		t.Fatalf("prompt = %q, want inspect", got.Run.Prompt)
	}
	if got.Run.Timeout != 3*time.Second {
		t.Fatalf("timeout = %s, want 3s", got.Run.Timeout)
	}
	if got.Run.PollInterval != 250*time.Millisecond {
		t.Fatalf("poll interval = %s, want 250ms", got.Run.PollInterval)
	}
	if !got.Output.Debug {
		t.Fatal("debug flag = false, want true")
	}
	assertFiles(t, got.Run.Files, []string{"README.md", "internal/application/workflow/worker.go"})
}

func TestParseRunNoWait(t *testing.T) {
	got, err := Parse([]string{commandRunName, "--prompt", "queue", "--no-wait"})
	if err != nil {
		t.Fatalf("parse run command: %v", err)
	}
	if got.Run.Wait {
		t.Fatal("wait = true, want false")
	}
}

func TestParseGetCommand(t *testing.T) {
	got, err := Parse([]string{commandGetName, "run_123", "--json"})
	if err != nil {
		t.Fatalf("parse get command: %v", err)
	}
	if got.Kind != CommandGet || got.RunID != "run_123" {
		t.Fatalf("parsed command = %#v", got)
	}
	if !got.Output.JSON {
		t.Fatal("json flag = false, want true")
	}
}

func TestParseListCommand(t *testing.T) {
	got, err := Parse([]string{commandListName, "--pretty"})
	if err != nil {
		t.Fatalf("parse list command: %v", err)
	}
	if got.Kind != CommandList {
		t.Fatalf("command = %q, want %q", got.Kind, CommandList)
	}
	if !got.Output.Pretty {
		t.Fatal("pretty flag = false, want true")
	}
}

func TestParseCancelCommand(t *testing.T) {
	got, err := Parse([]string{commandCancelName, "run_cancel"})
	if err != nil {
		t.Fatalf("parse cancel command: %v", err)
	}
	if got.Kind != CommandCancel || got.RunID != "run_cancel" {
		t.Fatalf("parsed command = %#v", got)
	}
}

func TestParseUsesAddressEnvFallback(t *testing.T) {
	t.Setenv(envCLIAddr, "http://example.test")

	got, err := Parse([]string{commandListName})
	if err != nil {
		t.Fatalf("parse list command: %v", err)
	}
	if got.Addr != "http://example.test" {
		t.Fatalf("addr = %q, want env fallback", got.Addr)
	}
}

func TestParseInvalidCommand(t *testing.T) {
	_, err := Parse([]string{"bad"})
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("Parse() error = %v, want %v", err, ErrUsage)
	}
}

func TestParseInvalidArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "empty", args: nil},
		{name: "run missing prompt", args: []string{commandRunName}},
		{name: "get missing id", args: []string{commandGetName}},
		{name: "list extra arg", args: []string{commandListName, "extra"}},
		{name: "cancel missing id", args: []string{commandCancelName}},
		{name: "server extra arg", args: []string{commandServerName, "extra"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.args)
			if !errors.Is(err, ErrUsage) {
				t.Fatalf("Parse() error = %v, want %v", err, ErrUsage)
			}
		})
	}
}

func TestUsage(t *testing.T) {
	got := Usage()
	for _, want := range []string{commandRunName, commandGetName, commandListName, commandCancelName} {
		if !contains(got, want) {
			t.Fatalf("usage = %q, want command %q", got, want)
		}
	}
}

func assertFiles(t *testing.T, got []string, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("files = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("files = %#v, want %#v", got, want)
		}
	}
}

func contains(value string, substring string) bool {
	return len(substring) == 0 || len(value) >= len(substring) && findSubstring(value, substring)
}

func findSubstring(value string, substring string) bool {
	for index := 0; index+len(substring) <= len(value); index++ {
		if value[index:index+len(substring)] == substring {
			return true
		}
	}

	return false
}
