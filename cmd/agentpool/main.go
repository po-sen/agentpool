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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app := bootstrap.New(version, os.Stderr)

	switch command {
	case cli.CommandServer:
		err = app.RunServer(ctx)
	case cli.CommandWorker:
		err = app.RunWorker(ctx)
	case cli.CommandDev:
		err = app.RunDev(ctx)
	case cli.CommandVersion:
		_, err = fmt.Fprintln(os.Stdout, app.Version())
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
