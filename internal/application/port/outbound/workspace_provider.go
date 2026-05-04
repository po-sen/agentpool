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
	Path     string
	BasePath string
}

// WorkspaceProvider resolves workspace sources requested by runs.
type WorkspaceProvider interface {
	ResolveWorkspace(context.Context, WorkspaceResolveRequest) (Workspace, error)
	CleanupWorkspace(context.Context, Workspace) error
}

const (
	// WorkspaceChangeAdded means a workspace file was created during a run.
	WorkspaceChangeAdded = "added"
	// WorkspaceChangeModified means a workspace file changed during a run.
	WorkspaceChangeModified = "modified"
	// WorkspaceChangeDeleted means a workspace file was removed during a run.
	WorkspaceChangeDeleted = "deleted"
)

// WorkspaceChange describes one file-level workspace change.
type WorkspaceChange struct {
	Path   string
	Status string
}

// WorkspaceChangeCollector compares a workspace against its baseline.
type WorkspaceChangeCollector interface {
	CollectWorkspaceChanges(context.Context, Workspace) ([]WorkspaceChange, error)
}
