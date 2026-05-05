package outbound

import (
	"context"

	"github.com/po-sen/agentpool/internal/domain/run"
)

// WorkspacePrepareRequest contains run attachments to materialize into a runtime workspace.
type WorkspacePrepareRequest struct {
	RunID       run.RunID
	Attachments []run.TaskAttachment
}

// Workspace describes a prepared runtime workspace.
type Workspace struct {
	Path     string
	HasFiles bool
}

// WorkspaceProvider prepares and cleans up ephemeral run workspaces.
type WorkspaceProvider interface {
	PrepareWorkspace(context.Context, WorkspacePrepareRequest) (Workspace, error)
	CleanupWorkspace(context.Context, Workspace) error
}
