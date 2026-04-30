package run

import "fmt"

// Status describes the lifecycle state of a run.
type Status string

const (
	// StatusQueued means the run has been accepted but not started.
	StatusQueued Status = "queued"
	// StatusPreparing means a worker is preparing execution resources.
	StatusPreparing Status = "preparing"
	// StatusRunning means the agent is actively executing the task.
	StatusRunning Status = "running"
	// StatusWaitingApproval means execution is paused for a human decision.
	StatusWaitingApproval Status = "waiting_approval"
	// StatusCompleted means execution finished successfully.
	StatusCompleted Status = "completed"
	// StatusFailed means execution ended with an error.
	StatusFailed Status = "failed"
	// StatusCancelled means execution was cancelled before completion.
	StatusCancelled Status = "cancelled"
)

// IsTerminal reports whether no further lifecycle transition is allowed.
func (s Status) IsTerminal() bool {
	return s == StatusCompleted || s == StatusFailed || s == StatusCancelled
}

// IsValid reports whether the status is part of the AgentPool lifecycle.
func (s Status) IsValid() bool {
	switch s {
	case StatusQueued,
		StatusPreparing,
		StatusRunning,
		StatusWaitingApproval,
		StatusCompleted,
		StatusFailed,
		StatusCancelled:
		return true
	default:
		return false
	}
}

func canTransition(from Status, to Status) bool {
	if from == to {
		return true
	}

	switch from {
	case StatusQueued:
		return to == StatusPreparing || to == StatusRunning || to == StatusCancelled
	case StatusPreparing:
		return to == StatusRunning || to == StatusFailed || to == StatusCancelled
	case StatusRunning:
		return to == StatusWaitingApproval ||
			to == StatusCompleted ||
			to == StatusFailed ||
			to == StatusCancelled
	case StatusWaitingApproval:
		return to == StatusRunning ||
			to == StatusCompleted ||
			to == StatusFailed ||
			to == StatusCancelled
	case StatusCompleted, StatusFailed, StatusCancelled:
		return false
	default:
		return false
	}
}

func invalidTransitionError(from Status, to Status) error {
	return fmt.Errorf("%w: %s to %s", ErrInvalidTransition, from, to)
}
