package inbound

import (
	"time"
)

// RunView is the application-level output contract for run use cases.
type RunView struct {
	ID        string
	Status    string
	Task      TaskView
	Steps     []StepView
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TaskView is the application-level representation of submitted task data.
type TaskView struct {
	ProjectID     string
	Prompt        string
	RepositoryURL string
	Branch        string
}

// StepView is the application-level representation of a run step.
type StepView struct {
	Name      string
	Status    string
	Message   string
	StartedAt time.Time
	EndedAt   *time.Time
}
