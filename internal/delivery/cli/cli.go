package cli

import (
	"errors"
	"flag"
	"io"
	"os"
	"strings"
	"time"
)

const (
	commandServerName  = "server"
	commandWorkerName  = "worker"
	commandDevName     = "dev"
	commandVersionName = "version"
	commandRunName     = "run"
	commandGetName     = "get"
	commandListName    = "list"
	commandCancelName  = "cancel"

	flagAddr         = "addr"
	flagDebug        = "debug"
	flagFile         = "file"
	flagJSON         = "json"
	flagNoWait       = "no-wait"
	flagPollInterval = "poll-interval"
	flagPretty       = "pretty"
	flagPrompt       = "prompt"
	flagTimeout      = "timeout"
	flagWait         = "wait"

	envCLIAddr = "AGENTPOOL_CLI_ADDR"

	defaultCLIAddr      = "http://localhost:8080"
	defaultPollInterval = 500 * time.Millisecond
	defaultRunTimeout   = 2 * time.Minute
)

// CommandKind identifies a supported top-level CLI command.
type CommandKind string

const (
	// CommandServer starts the HTTP server.
	CommandServer CommandKind = commandServerName
	// CommandWorker starts the worker process.
	CommandWorker CommandKind = commandWorkerName
	// CommandDev starts the HTTP server and worker in one process.
	CommandDev CommandKind = commandDevName
	// CommandVersion prints version information.
	CommandVersion CommandKind = commandVersionName
	// CommandRun submits a run through the HTTP API.
	CommandRun CommandKind = commandRunName
	// CommandGet fetches one run through the HTTP API.
	CommandGet CommandKind = commandGetName
	// CommandList lists runs through the HTTP API.
	CommandList CommandKind = commandListName
	// CommandCancel cancels one run through the HTTP API.
	CommandCancel CommandKind = commandCancelName
)

// Command is the parsed CLI command and its command-specific options.
type Command struct {
	Kind   CommandKind
	Addr   string
	RunID  string
	Run    RunOptions
	Output OutputOptions
}

// RunOptions controls run submission and polling.
type RunOptions struct {
	Prompt       string
	Files        []string
	Wait         bool
	Timeout      time.Duration
	PollInterval time.Duration
}

// OutputOptions controls CLI response formatting.
type OutputOptions struct {
	JSON   bool
	Pretty bool
	Debug  bool
}

// ErrUsage indicates invalid CLI usage.
var ErrUsage = errors.New("invalid command")

// Parse returns the requested top-level command and parsed options.
func Parse(args []string) (Command, error) {
	if len(args) == 0 {
		return Command{}, ErrUsage
	}

	switch args[0] {
	case commandServerName:
		return parseNoArgCommand(CommandServer, args[1:])
	case commandWorkerName:
		return parseNoArgCommand(CommandWorker, args[1:])
	case commandDevName:
		return parseNoArgCommand(CommandDev, args[1:])
	case commandVersionName:
		return parseNoArgCommand(CommandVersion, args[1:])
	case commandRunName:
		return parseRunCommand(args[1:])
	case commandGetName:
		return parseRunIDCommand(CommandGet, args[1:])
	case commandListName:
		return parseListCommand(args[1:])
	case commandCancelName:
		return parseRunIDCommand(CommandCancel, args[1:])
	default:
		return Command{}, ErrUsage
	}
}

// UsesHTTPClient reports whether the command should execute as an HTTP client command.
func (c Command) UsesHTTPClient() bool {
	return c.Kind == CommandRun ||
		c.Kind == CommandGet ||
		c.Kind == CommandList ||
		c.Kind == CommandCancel
}

// Usage returns the short CLI usage text.
func Usage() string {
	return strings.TrimSpace(`
usage:
  agentpool server
  agentpool worker
  agentpool dev
  agentpool version
  agentpool run --prompt "..." [--file PATH ...] [--addr URL] [--json|--pretty] [--debug]
  agentpool get <run_id> [--addr URL] [--json|--pretty] [--debug]
  agentpool list [--addr URL] [--json|--pretty] [--debug]
  agentpool cancel <run_id> [--addr URL] [--json|--pretty] [--debug]
`) + "\n"
}

func parseNoArgCommand(kind CommandKind, args []string) (Command, error) {
	if len(args) != 0 {
		return Command{}, ErrUsage
	}

	return Command{Kind: kind}, nil
}

func parseRunCommand(args []string) (Command, error) {
	command := newClientCommand(CommandRun)
	files := fileListFlag{}
	noWait := false
	flags := newFlagSet(commandRunName)
	flags.StringVar(&command.Run.Prompt, flagPrompt, "", "run prompt")
	flags.Var(&files, flagFile, "file to upload")
	flags.BoolVar(&command.Run.Wait, flagWait, true, "wait for terminal run status")
	flags.BoolVar(&noWait, flagNoWait, false, "submit and return without polling")
	flags.DurationVar(&command.Run.Timeout, flagTimeout, defaultRunTimeout, "maximum wait duration")
	flags.DurationVar(&command.Run.PollInterval, flagPollInterval, defaultPollInterval, "poll interval")
	addClientFlags(flags, &command)

	if err := flags.Parse(args); err != nil {
		return Command{}, ErrUsage
	}
	if flags.NArg() != 0 || strings.TrimSpace(command.Run.Prompt) == "" {
		return Command{}, ErrUsage
	}
	if noWait {
		command.Run.Wait = false
	}
	command.Run.Files = append([]string(nil), files...)

	return command, nil
}

func parseRunIDCommand(kind CommandKind, args []string) (Command, error) {
	command := newClientCommand(kind)
	flags := newFlagSet(string(kind))
	addClientFlags(flags, &command)

	flagArgs, runID, err := splitRunIDArgs(args)
	if err != nil {
		return Command{}, ErrUsage
	}
	if err := flags.Parse(flagArgs); err != nil {
		return Command{}, ErrUsage
	}
	if flags.NArg() != 0 || strings.TrimSpace(runID) == "" {
		return Command{}, ErrUsage
	}
	command.RunID = runID

	return command, nil
}

func parseListCommand(args []string) (Command, error) {
	command := newClientCommand(CommandList)
	flags := newFlagSet(commandListName)
	addClientFlags(flags, &command)

	if err := flags.Parse(args); err != nil {
		return Command{}, ErrUsage
	}
	if flags.NArg() != 0 {
		return Command{}, ErrUsage
	}

	return command, nil
}

func newClientCommand(kind CommandKind) Command {
	return Command{
		Kind: kind,
		Addr: defaultAddress(),
		Run: RunOptions{
			Wait:         true,
			Timeout:      defaultRunTimeout,
			PollInterval: defaultPollInterval,
		},
	}
}

func defaultAddress() string {
	if value := strings.TrimSpace(os.Getenv(envCLIAddr)); value != "" {
		return value
	}

	return defaultCLIAddr
}

func newFlagSet(name string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	return flags
}

func addClientFlags(flags *flag.FlagSet, command *Command) {
	flags.StringVar(&command.Addr, flagAddr, command.Addr, "AgentPool HTTP API address")
	flags.BoolVar(&command.Output.JSON, flagJSON, false, "print full JSON response")
	flags.BoolVar(&command.Output.Pretty, flagPretty, false, "print human-readable response")
	flags.BoolVar(&command.Output.Debug, flagDebug, false, "include debug diagnostics")
}

func splitRunIDArgs(args []string) ([]string, string, error) {
	flagArgs := make([]string, 0, len(args))
	runID := ""
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if isFlagArg(arg) {
			flagArgs = append(flagArgs, arg)
			if flagName(arg) == flagAddr && !strings.Contains(arg, "=") {
				index++
				if index >= len(args) {
					return nil, "", ErrUsage
				}
				flagArgs = append(flagArgs, args[index])
			}
			continue
		}
		if runID != "" {
			return nil, "", ErrUsage
		}
		runID = arg
	}

	return flagArgs, runID, nil
}

func isFlagArg(arg string) bool {
	return strings.HasPrefix(arg, "-") && arg != "-"
}

func flagName(arg string) string {
	name := strings.TrimLeft(arg, "-")
	if index := strings.Index(name, "="); index >= 0 {
		name = name[:index]
	}

	return name
}

type fileListFlag []string

func (f *fileListFlag) String() string {
	return strings.Join(*f, ",")
}

func (f *fileListFlag) Set(value string) error {
	if strings.TrimSpace(value) == "" {
		return ErrUsage
	}
	*f = append(*f, value)

	return nil
}
