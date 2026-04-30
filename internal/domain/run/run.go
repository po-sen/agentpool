package run

import "time"

// RunID identifies an AgentPool run.
//
//nolint:revive // RunID is part of the domain ubiquitous language.
type RunID string

// String returns the plain string form of the run ID.
func (id RunID) String() string {
	return string(id)
}

// Run is the aggregate root for a submitted agent task.
type Run struct {
	ID        RunID
	Task      TaskSpec
	Status    Status
	Steps     []Step
	CreatedAt time.Time
	UpdatedAt time.Time
}

// New creates a queued run.
func New(id RunID, task TaskSpec, now time.Time) (*Run, error) {
	if id == "" {
		return nil, ErrEmptyRunID
	}
	if err := task.Validate(); err != nil {
		return nil, err
	}

	return &Run{
		ID:        id,
		Task:      task,
		Status:    StatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// Clone returns a detached copy of the run.
func (r *Run) Clone() *Run {
	if r == nil {
		return nil
	}

	clone := *r
	if len(r.Steps) > 0 {
		clone.Steps = append([]Step(nil), r.Steps...)
	}

	return &clone
}

// TransitionTo moves the run to the next lifecycle status when allowed.
func (r *Run) TransitionTo(next Status, now time.Time) error {
	if !next.IsValid() {
		return ErrUnknownStatus
	}
	if !canTransition(r.Status, next) {
		return invalidTransitionError(r.Status, next)
	}

	r.Status = next
	r.UpdatedAt = now

	return nil
}

// CanCancel reports whether the run can still be cancelled.
func (r *Run) CanCancel() bool {
	return !r.Status.IsTerminal()
}

// Cancel moves the run into the cancelled terminal state.
func (r *Run) Cancel(now time.Time) error {
	if !r.CanCancel() {
		return invalidTransitionError(r.Status, StatusCancelled)
	}

	return r.TransitionTo(StatusCancelled, now)
}

// StartPreparing marks the run as preparing resources.
func (r *Run) StartPreparing(now time.Time) error {
	return r.TransitionTo(StatusPreparing, now)
}

// StartRunning marks the run as actively running.
func (r *Run) StartRunning(now time.Time) error {
	return r.TransitionTo(StatusRunning, now)
}

// WaitForApproval pauses the run for a human decision.
func (r *Run) WaitForApproval(now time.Time) error {
	return r.TransitionTo(StatusWaitingApproval, now)
}

// Complete marks the run as successfully completed.
func (r *Run) Complete(now time.Time) error {
	return r.TransitionTo(StatusCompleted, now)
}

// Fail marks the run as failed.
func (r *Run) Fail(now time.Time) error {
	return r.TransitionTo(StatusFailed, now)
}
