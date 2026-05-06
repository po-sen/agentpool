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

// Workspace describes a prepared mutable runtime workspace.
type Workspace struct {
	RootPath  string
	StatePath string
	Sources   []WorkspaceSource
}

// WorkspaceSource describes an already-authorized input that can be materialized into a workspace.
type WorkspaceSource struct {
	ID        string
	Path      string
	MediaType string
	SizeBytes int64
	Checksum  string
}

// WorkspaceMaterializeRequest asks a provider to copy an authorized source into the workspace.
type WorkspaceMaterializeRequest struct {
	Workspace Workspace
	SourceID  string
	Path      string
	Overwrite bool
}

// WorkspaceStagedFile describes a workspace file materialized from an authorized source.
type WorkspaceStagedFile struct {
	SourceID  string
	Path      string
	SizeBytes int64
	Checksum  string
}

// WorkspaceProvider prepares and cleans up ephemeral run workspaces.
type WorkspaceProvider interface {
	PrepareWorkspace(context.Context, WorkspacePrepareRequest) (Workspace, error)
	CollectArtifacts(context.Context, Workspace) ([]run.Artifact, error)
	CleanupWorkspace(context.Context, Workspace) error
}

// WorkspaceMaterializer materializes authorized input sources into an existing workspace.
type WorkspaceMaterializer interface {
	MaterializeWorkspaceSource(context.Context, WorkspaceMaterializeRequest) (WorkspaceStagedFile, error)
	ListWorkspaceStagedFiles(context.Context, Workspace) ([]WorkspaceStagedFile, error)
}
