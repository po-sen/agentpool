package outbound

import (
	"context"

	"github.com/po-sen/agentpool/internal/domain/run"
)

// WorkspaceResolveRequest contains context needed to resolve a run workspace source.
type WorkspaceResolveRequest struct {
	RunID  run.RunID
	Task   run.TaskSpec
	Source run.WorkspaceSource
}

// Workspace describes a resolved workspace.
type Workspace struct {
	Path string
}

// WorkspaceProvider resolves workspace sources requested by runs.
type WorkspaceProvider interface {
	ResolveWorkspace(context.Context, WorkspaceResolveRequest) (Workspace, error)
}
