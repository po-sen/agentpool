package run

import (
	"errors"
	"testing"
	"time"
)

func TestStepCapturesExecutionProgress(t *testing.T) {
	startedAt := time.Unix(100, 0).UTC()
	endedAt := time.Unix(101, 0).UTC()

	step := Step{
		Name:      "execute",
		Status:    StatusCompleted,
		Message:   "done",
		StartedAt: startedAt,
		EndedAt:   endedAt,
	}

	if step.Name != "execute" {
		t.Fatalf("Name = %s, want execute", step.Name)
	}
	if step.Status != StatusCompleted {
		t.Fatalf("Status = %s, want %s", step.Status, StatusCompleted)
	}
	if !step.EndedAt.Equal(endedAt) {
		t.Fatalf("EndedAt = %v, want %v", step.EndedAt, endedAt)
	}
}

func TestRunStartStepAppendsRunningStep(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	item := newRun(t, now)

	err := item.StartStep("prepare", "Preparing execution", now.Add(time.Second))
	if err != nil {
		t.Fatalf("start step: %v", err)
	}

	if len(item.Steps) != 1 {
		t.Fatalf("len(Steps) = %d, want 1", len(item.Steps))
	}
	step := item.Steps[0]
	if step.Name != "prepare" {
		t.Fatalf("Name = %q, want prepare", step.Name)
	}
	if step.Status != StatusRunning {
		t.Fatalf("Status = %s, want %s", step.Status, StatusRunning)
	}
	if step.Message != "Preparing execution" {
		t.Fatalf("Message = %q, want Preparing execution", step.Message)
	}
	if !step.StartedAt.Equal(now.Add(time.Second)) {
		t.Fatalf("StartedAt = %v, want %v", step.StartedAt, now.Add(time.Second))
	}
	if !step.EndedAt.IsZero() {
		t.Fatalf("EndedAt = %v, want zero", step.EndedAt)
	}
}

func TestRunCompleteStepCompletesLatestMatchingRunningStep(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	item := newRun(t, now)
	if err := item.StartStep("agent", "first", now.Add(time.Second)); err != nil {
		t.Fatalf("start first step: %v", err)
	}
	if err := item.CompleteStep("agent", "first complete", now.Add(2*time.Second)); err != nil {
		t.Fatalf("complete first step: %v", err)
	}
	if err := item.StartStep("agent", "second", now.Add(3*time.Second)); err != nil {
		t.Fatalf("start second step: %v", err)
	}

	err := item.CompleteStep("agent", "Agent generated result summary", now.Add(4*time.Second))
	if err != nil {
		t.Fatalf("complete second step: %v", err)
	}

	if item.Steps[0].Message != "first complete" {
		t.Fatalf("first step message = %q, want first complete", item.Steps[0].Message)
	}
	step := item.Steps[1]
	if step.Status != StatusCompleted {
		t.Fatalf("second step status = %s, want %s", step.Status, StatusCompleted)
	}
	if step.Message != "Agent generated result summary" {
		t.Fatalf("second step message = %q, want Agent generated result summary", step.Message)
	}
	if !step.EndedAt.Equal(now.Add(4 * time.Second)) {
		t.Fatalf("EndedAt = %v, want %v", step.EndedAt, now.Add(4*time.Second))
	}
}

func TestRunFailStepFailsMatchingRunningStep(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	item := newRun(t, now)
	if err := item.StartStep("agent", "Agent execution started", now.Add(time.Second)); err != nil {
		t.Fatalf("start step: %v", err)
	}

	err := item.FailStep("agent", "Agent execution failed", now.Add(2*time.Second))
	if err != nil {
		t.Fatalf("fail step: %v", err)
	}

	step := item.Steps[0]
	if step.Status != StatusFailed {
		t.Fatalf("Status = %s, want %s", step.Status, StatusFailed)
	}
	if step.Message != "Agent execution failed" {
		t.Fatalf("Message = %q, want Agent execution failed", step.Message)
	}
	if !step.EndedAt.Equal(now.Add(2 * time.Second)) {
		t.Fatalf("EndedAt = %v, want %v", step.EndedAt, now.Add(2*time.Second))
	}
}

func TestRunCompleteStepReturnsErrorWithoutMatchingRunningStep(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	item := newRun(t, now)

	err := item.CompleteStep("agent", "done", now.Add(time.Second))
	if !errors.Is(err, ErrStepNotFound) {
		t.Fatalf("CompleteStep() error = %v, want %v", err, ErrStepNotFound)
	}

	if err := item.StartStep("agent", "running", now.Add(2*time.Second)); err != nil {
		t.Fatalf("start step: %v", err)
	}
	if err := item.CompleteStep("agent", "done", now.Add(3*time.Second)); err != nil {
		t.Fatalf("complete step: %v", err)
	}

	err = item.CompleteStep("agent", "done again", now.Add(4*time.Second))
	if !errors.Is(err, ErrStepAlreadyEnded) {
		t.Fatalf("CompleteStep() error = %v, want %v", err, ErrStepAlreadyEnded)
	}
}

func TestRunStartStepRejectsInvalidName(t *testing.T) {
	item := newRun(t, time.Unix(100, 0).UTC())

	err := item.StartStep(" ", "message", time.Unix(101, 0).UTC())
	if !errors.Is(err, ErrInvalidStepName) {
		t.Fatalf("StartStep() error = %v, want %v", err, ErrInvalidStepName)
	}
}
