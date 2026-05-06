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
		"--dir",
		".",
		"--archive",
		"project.tar.gz",
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
	assertFiles(t, got.Run.Dirs, []string{"."})
	assertFiles(t, got.Run.Archives, []string{"project.tar.gz"})
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

func TestParseRunWatch(t *testing.T) {
	got, err := Parse([]string{commandRunName, "--prompt", "inspect", "--watch"})
	if err != nil {
		t.Fatalf("parse run command: %v", err)
	}
	if !got.Run.Watch {
		t.Fatal("watch = false, want true")
	}
	if !got.Run.Wait {
		t.Fatal("wait = false, want true")
	}
}

func TestParseRunWatchRejectsNoWait(t *testing.T) {
	_, err := Parse([]string{commandRunName, "--prompt", "inspect", "--watch", "--no-wait"})
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("Parse() error = %v, want %v", err, ErrUsage)
	}
}

func TestParseWatchCommand(t *testing.T) {
	got, err := Parse([]string{
		commandWatchName,
		"run_123",
		"--debug",
		"--timeout",
		"5s",
		"--poll-interval",
		"100ms",
	})
	if err != nil {
		t.Fatalf("parse watch command: %v", err)
	}
	if got.Kind != CommandWatch || got.RunID != "run_123" {
		t.Fatalf("parsed command = %#v", got)
	}
	if !got.Run.Watch || !got.Output.Debug {
		t.Fatalf("watch/debug flags = %v/%v, want true/true", got.Run.Watch, got.Output.Debug)
	}
	if got.Run.Timeout != 5*time.Second {
		t.Fatalf("timeout = %s, want 5s", got.Run.Timeout)
	}
	if got.Run.PollInterval != 100*time.Millisecond {
		t.Fatalf("poll interval = %s, want 100ms", got.Run.PollInterval)
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

func TestParseArtifactCommands(t *testing.T) {
	list, err := Parse([]string{commandArtifactsName, "run_test", "--json"})
	if err != nil {
		t.Fatalf("parse artifacts command: %v", err)
	}
	if list.Kind != CommandArtifacts || list.RunID != "run_test" {
		t.Fatalf("parsed artifacts command = %#v", list)
	}
	if !list.Output.JSON {
		t.Fatal("artifacts json flag = false, want true")
	}

	get, err := Parse([]string{commandArtifactName, "run_test", "reports/report.md"})
	if err != nil {
		t.Fatalf("parse artifact command: %v", err)
	}
	if get.Kind != CommandArtifact || get.RunID != "run_test" || get.Path != "reports/report.md" {
		t.Fatalf("parsed artifact command = %#v", get)
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
		{name: "watch missing id", args: []string{commandWatchName}},
		{name: "get missing id", args: []string{commandGetName}},
		{name: "list extra arg", args: []string{commandListName, "extra"}},
		{name: "cancel missing id", args: []string{commandCancelName}},
		{name: "artifacts missing id", args: []string{commandArtifactsName}},
		{name: "artifact missing path", args: []string{commandArtifactName, "run_test"}},
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
	for _, want := range []string{commandRunName, commandWatchName, commandGetName, commandListName, commandCancelName} {
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
