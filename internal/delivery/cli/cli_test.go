package cli_test

import (
	"errors"
	"testing"

	"github.com/po-sen/agentpool/internal/delivery/cli"
)

func TestParseValidCommands(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want cli.Command
	}{
		{name: "server", arg: "server", want: cli.CommandServer},
		{name: "worker", arg: "worker", want: cli.CommandWorker},
		{name: "dev", arg: "dev", want: cli.CommandDev},
		{name: "version", arg: "version", want: cli.CommandVersion},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cli.Parse([]string{tt.arg})
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
	_, err := cli.Parse([]string{"bad"})
	if !errors.Is(err, cli.ErrUsage) {
		t.Fatalf("Parse() error = %v, want %v", err, cli.ErrUsage)
	}
}

func TestUsage(t *testing.T) {
	const want = "usage: agentpool <server|worker|dev|version>\n"
	if got := cli.Usage(); got != want {
		t.Fatalf("usage = %q, want %q", got, want)
	}
}
