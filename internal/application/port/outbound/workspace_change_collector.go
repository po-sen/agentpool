package outbound

import "context"

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
