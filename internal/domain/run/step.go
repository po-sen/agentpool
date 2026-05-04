package run

import (
	"strings"
	"time"
)

const cancelledStepMessage = "Run cancelled"

// Step describes one coarse execution step in a run.
type Step struct {
	Name      string
	Status    Status
	Message   string
	StartedAt time.Time
	EndedAt   time.Time
}

// StartStep appends a running execution timeline step to the run.
func (r *Run) StartStep(name string, message string, now time.Time) error {
	if strings.TrimSpace(name) == "" {
		return ErrInvalidStepName
	}

	r.Steps = append(r.Steps, Step{
		Name:      name,
		Status:    StatusRunning,
		Message:   message,
		StartedAt: now,
	})
	r.UpdatedAt = now

	return nil
}

// CompleteStep marks the latest matching running execution timeline step as completed.
func (r *Run) CompleteStep(name string, message string, now time.Time) error {
	return r.endStep(name, StatusCompleted, message, now)
}

// FailStep marks the latest matching running execution timeline step as failed.
func (r *Run) FailStep(name string, message string, now time.Time) error {
	return r.endStep(name, StatusFailed, message, now)
}

func (r *Run) cancelRunningSteps(now time.Time) {
	for i := range r.Steps {
		if r.Steps[i].Status != StatusRunning {
			continue
		}

		r.Steps[i].Status = StatusCancelled
		r.Steps[i].Message = cancelledStepMessage
		r.Steps[i].EndedAt = now
		r.UpdatedAt = now
	}
}

func (r *Run) endStep(name string, status Status, message string, now time.Time) error {
	if strings.TrimSpace(name) == "" {
		return ErrInvalidStepName
	}

	foundMatchingStep := false
	for i := len(r.Steps) - 1; i >= 0; i-- {
		if r.Steps[i].Name != name {
			continue
		}
		foundMatchingStep = true
		if r.Steps[i].Status != StatusRunning {
			continue
		}

		r.Steps[i].Status = status
		r.Steps[i].Message = message
		r.Steps[i].EndedAt = now
		r.UpdatedAt = now

		return nil
	}

	if foundMatchingStep {
		return ErrStepAlreadyEnded
	}

	return ErrStepNotFound
}
