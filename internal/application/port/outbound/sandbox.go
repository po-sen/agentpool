package outbound

import (
	"context"

	"github.com/po-sen/agentpool/internal/domain/run"
)

// Sandbox describes a prepared execution environment.
type Sandbox struct {
	ID               string
	SupportsCommands bool
}

// SandboxCapabilities describes execution capabilities available from a sandbox provider.
type SandboxCapabilities struct {
	SupportsCommands bool
}

// SandboxRequest contains information needed to prepare a sandbox.
type SandboxRequest struct {
	RunID     run.RunID
	Task      run.TaskSpec
	Workspace Workspace
}

// SandboxProvider prepares and cleans up execution sandboxes.
type SandboxProvider interface {
	Prepare(context.Context, SandboxRequest) (Sandbox, error)
	Cleanup(context.Context, Sandbox) error
}

// SandboxCapabilityProvider reports capabilities a sandbox provider can prepare.
type SandboxCapabilityProvider interface {
	Capabilities() SandboxCapabilities
}
