package cli

import (
	"errors"
	"testing"
)

func TestParseValidCommands(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want Command
	}{
		{name: "server", arg: "server", want: CommandServer},
		{name: "worker", arg: "worker", want: CommandWorker},
		{name: "dev", arg: "dev", want: CommandDev},
		{name: "version", arg: "version", want: CommandVersion},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse([]string{tt.arg})
			if err != nil {
				t.Fatalf("parse command: %v", err)
			}
			if got != tt.want {
				t.Fatalf("command = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseInvalidCommand(t *testing.T) {
	_, err := Parse([]string{"bad"})
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("Parse() error = %v, want %v", err, ErrUsage)
	}
}

func TestUsage(t *testing.T) {
	const want = "usage: agentpool <server|worker|dev|version>\n"
	if got := Usage(); got != want {
		t.Fatalf("usage = %q, want %q", got, want)
	}
}
