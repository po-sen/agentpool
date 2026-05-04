package inbound

import (
	"time"
)

// RunView is the application-level output contract for run use cases.
type RunView struct {
	ID               string
	Status           string
	Task             TaskView
	Result           RunResultView
	FailureReason    string
	WorkspaceChanges []WorkspaceChangeView
	Steps            []StepView
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// RunResultView is the application-level representation of successful run output.
type RunResultView struct {
	Summary string
}

// TaskView is the application-level representation of submitted task data.
type TaskView struct {
	ProjectID     string
	Prompt        string
	RepositoryURL string
	Branch        string
	Workspace     WorkspaceSourceView
}

// WorkspaceSourceView is the application-level representation of run workspace intent.
type WorkspaceSourceView struct {
	Type       string
	SnapshotID string
}

// WorkspaceChangeView is the application-level representation of a workspace file change.
type WorkspaceChangeView struct {
	Path   string
	Status string
}

// StepView is the application-level representation of a run step.
type StepView struct {
	Name      string
	Status    string
	Message   string
	StartedAt time.Time
	EndedAt   *time.Time
}
