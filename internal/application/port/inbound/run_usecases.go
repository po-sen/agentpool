package inbound

import (
	"context"
)

// CreateRunCommand contains application-level input for creating a run.
type CreateRunCommand struct {
	ProjectID     string
	Prompt        string
	RepositoryURL string
	Branch        string
	Workspace     WorkspaceSourceInput
}

// WorkspaceSourceInput contains application-level workspace source input.
type WorkspaceSourceInput struct {
	Type       string
	SnapshotID string
}

// CancelRunCommand contains application-level input for cancelling a run.
type CancelRunCommand struct {
	RunID string
}

// GetRunQuery contains application-level input for looking up a run.
type GetRunQuery struct {
	RunID string
}

// ApproveRunCommand contains application-level input for an approval decision.
type ApproveRunCommand struct {
	RunID    string
	Decision string
}

// CreateRunUseCase accepts a new run submission.
type CreateRunUseCase interface {
	CreateRun(context.Context, CreateRunCommand) (RunView, error)
}

// GetRunUseCase retrieves a run by ID.
type GetRunUseCase interface {
	GetRun(context.Context, GetRunQuery) (RunView, error)
}

// ListRunsUseCase retrieves submitted runs.
type ListRunsUseCase interface {
	ListRuns(context.Context) ([]RunView, error)
}

// ApproveRunUseCase records a human approval decision.
type ApproveRunUseCase interface {
	ApproveRun(context.Context, ApproveRunCommand) (RunView, error)
}

// CancelRunUseCase cancels a run that has not reached a terminal status.
type CancelRunUseCase interface {
	CancelRun(context.Context, CancelRunCommand) (RunView, error)
}
