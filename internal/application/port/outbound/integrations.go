package outbound

import (
	"context"

	"github.com/po-sen/agentpool/internal/domain/run"
)

// IDGenerator creates run IDs.
type IDGenerator interface {
	NewRunID() (run.RunID, error)
}

// Sandbox describes a prepared execution environment.
type Sandbox struct {
	ID            string
	WorkspacePath string
}

// SandboxRequest contains information needed to prepare a sandbox.
type SandboxRequest struct {
	RunID run.RunID
	Task  run.TaskSpec
}

// SandboxProvider prepares and cleans up execution sandboxes.
type SandboxProvider interface {
	Prepare(context.Context, SandboxRequest) (Sandbox, error)
	Cleanup(context.Context, Sandbox) error
}

// GitFetchRequest describes a source checkout request.
type GitFetchRequest struct {
	RepositoryURL string
	Branch        string
}

// GitCheckout describes a fetched source checkout.
type GitCheckout struct {
	Path string
}

// GitProvider provides source checkout operations.
type GitProvider interface {
	Fetch(context.Context, GitFetchRequest) (GitCheckout, error)
}

// PolicyDecisionRequest describes a policy evaluation request.
type PolicyDecisionRequest struct {
	RunID run.RunID
	Task  run.TaskSpec
}

// PolicyDecision describes whether a requested action is allowed.
type PolicyDecision struct {
	Allowed bool
	Reason  string
}

// PolicyDecisionPort evaluates policy decisions.
type PolicyDecisionPort interface {
	Decide(context.Context, PolicyDecisionRequest) (PolicyDecision, error)
}

// SecretRequest describes a request for runtime secrets.
type SecretRequest struct {
	ProjectID string
	Names     []string
}

// SecretBundle contains resolved runtime secrets.
type SecretBundle struct {
	Values map[string]string
}

// SecretBroker resolves runtime secrets.
type SecretBroker interface {
	Resolve(context.Context, SecretRequest) (SecretBundle, error)
}
