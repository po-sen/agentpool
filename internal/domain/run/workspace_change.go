package run

// WorkspaceChangeStatus describes how a workspace file changed during a run.
type WorkspaceChangeStatus string

const (
	// WorkspaceChangeAdded means the run created a file.
	WorkspaceChangeAdded WorkspaceChangeStatus = "added"
	// WorkspaceChangeModified means the run changed an existing file.
	WorkspaceChangeModified WorkspaceChangeStatus = "modified"
	// WorkspaceChangeDeleted means the run removed a file.
	WorkspaceChangeDeleted WorkspaceChangeStatus = "deleted"
)

// WorkspaceChange records one changed workspace file.
type WorkspaceChange struct {
	Path   string
	Status WorkspaceChangeStatus
}
