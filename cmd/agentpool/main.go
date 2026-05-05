package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/po-sen/agentpool/internal/bootstrap"
	"github.com/po-sen/agentpool/internal/delivery/cli"
)

var version = "dev"

func main() {
	command, err := cli.Parse(os.Args[1:])
	if err != nil {
		writeStderr(cli.Usage())
		os.Exit(2)
	}
	if command.Kind == cli.CommandVersion {
		if _, err := fmt.Fprintf(os.Stdout, "agentpool %s\n", version); err != nil {
			writeStderr(err.Error() + "\n")
			os.Exit(1)
		}
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if command.UsesHTTPClient() {
		if err := cli.Execute(ctx, command, os.Stdout); err != nil {
			writeStderr(err.Error() + "\n")
			os.Exit(1)
		}
		return
	}

	app, err := bootstrap.New(version, os.Stderr)
	if err != nil {
		writeStderr(err.Error() + "\n")
		os.Exit(1)
	}

	switch command.Kind {
	case cli.CommandServer:
		err = app.RunServer(ctx)
	case cli.CommandWorker:
		err = app.RunWorker(ctx)
	case cli.CommandDev:
		err = app.RunDev(ctx)
	}

	if err != nil {
		writeStderr(err.Error() + "\n")
		os.Exit(1)
	}
}

func writeStderr(message string) {
	if _, err := fmt.Fprint(os.Stderr, message); err != nil {
		return
	}
}
