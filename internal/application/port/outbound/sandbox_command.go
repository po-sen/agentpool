package outbound

import (
	"context"
	"time"
)

// SandboxCommandRequest describes a command execution request inside a prepared sandbox.
type SandboxCommandRequest struct {
	Sandbox       Sandbox
	WorkspacePath string
	Command       string
	Timeout       time.Duration
}

// SandboxCommandResult describes the result of a sandbox command.
type SandboxCommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	TimedOut bool
}

// SandboxCommandRunner runs commands inside prepared sandboxes.
type SandboxCommandRunner interface {
	RunCommand(context.Context, SandboxCommandRequest) (SandboxCommandResult, error)
}
