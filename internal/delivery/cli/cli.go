package cli

import "errors"

// Command is a supported top-level CLI command.
type Command string

const (
	// CommandServer starts the HTTP server.
	CommandServer Command = "server"
	// CommandWorker starts the worker process.
	CommandWorker Command = "worker"
	// CommandDev starts the HTTP server and worker in one process.
	CommandDev Command = "dev"
	// CommandVersion prints version information.
	CommandVersion Command = "version"
)

// ErrUsage indicates invalid CLI usage.
var ErrUsage = errors.New("invalid command")

// Parse returns the requested top-level command.
func Parse(args []string) (Command, error) {
	if len(args) != 1 {
		return "", ErrUsage
	}

	switch Command(args[0]) {
	case CommandServer:
		return CommandServer, nil
	case CommandWorker:
		return CommandWorker, nil
	case CommandDev:
		return CommandDev, nil
	case CommandVersion:
		return CommandVersion, nil
	default:
		return "", ErrUsage
	}
}

// Usage returns the short CLI usage text.
func Usage() string {
	return "usage: agentpool <server|worker|dev|version>\n"
}
