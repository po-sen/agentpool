package run

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRunStatusTransitions(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	item := newRun(t, now)

	assertTransition(t, item.StartPreparing(now.Add(time.Second)))
	assertStatus(t, item, StatusPreparing)

	assertTransition(t, item.StartRunning(now.Add(2*time.Second)))
	assertStatus(t, item, StatusRunning)

	assertTransition(t, item.WaitForApproval(now.Add(3*time.Second)))
	assertStatus(t, item, StatusWaitingApproval)

	assertTransition(t, item.StartRunning(now.Add(4*time.Second)))
	assertStatus(t, item, StatusRunning)

	assertTransition(t, item.Complete(now.Add(5*time.Second)))
	assertStatus(t, item, StatusCompleted)
}

func TestRunRejectsInvalidTransition(t *testing.T) {
	item := newRun(t, time.Unix(100, 0).UTC())

	err := item.Complete(time.Unix(101, 0).UTC())
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected invalid transition error, got %v", err)
	}
	assertStatus(t, item, StatusQueued)
}

func TestRunCompleteWithResultStoresSummary(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	item := newRun(t, now)
	if err := item.StartRunning(now.Add(time.Second)); err != nil {
		t.Fatalf("start running: %v", err)
	}
	item.FailureReason = "previous failure"

	err := item.CompleteWithResult(now.Add(2*time.Second), "model output")
	if err != nil {
		t.Fatalf("complete with result: %v", err)
	}

	assertStatus(t, item, StatusCompleted)
	if item.ResultSummary != "model output" {
		t.Fatalf("ResultSummary = %q, want %q", item.ResultSummary, "model output")
	}
	if item.FailureReason != "" {
		t.Fatalf("FailureReason = %q, want empty", item.FailureReason)
	}
}

func TestRunFailWithReasonStoresFailureReason(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	item := newRun(t, now)
	if err := item.StartRunning(now.Add(time.Second)); err != nil {
		t.Fatalf("start running: %v", err)
	}
	item.ResultSummary = "stale result"

	err := item.FailWithReason(now.Add(2*time.Second), "model failed")
	if err != nil {
		t.Fatalf("fail with reason: %v", err)
	}

	assertStatus(t, item, StatusFailed)
	if item.FailureReason != "model failed" {
		t.Fatalf("FailureReason = %q, want %q", item.FailureReason, "model failed")
	}
	if item.ResultSummary != "" {
		t.Fatalf("ResultSummary = %q, want empty", item.ResultSummary)
	}
}

func TestRunResultMethodsRejectInvalidTransitionsWithoutMutatingOutput(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	item := newRun(t, now)

	err := item.CompleteWithResult(now.Add(time.Second), "model output")
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("CompleteWithResult() error = %v, want %v", err, ErrInvalidTransition)
	}
	assertStatus(t, item, StatusQueued)
	if item.ResultSummary != "" {
		t.Fatalf("ResultSummary = %q, want empty", item.ResultSummary)
	}

	item.ResultSummary = "completed output"
	if err := item.StartRunning(now.Add(2 * time.Second)); err != nil {
		t.Fatalf("start running: %v", err)
	}
	if err := item.CompleteWithResult(now.Add(3*time.Second), "completed output"); err != nil {
		t.Fatalf("complete with result: %v", err)
	}

	err = item.FailWithReason(now.Add(4*time.Second), "too late")
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("FailWithReason() error = %v, want %v", err, ErrInvalidTransition)
	}
	assertStatus(t, item, StatusCompleted)
	if item.ResultSummary != "completed output" {
		t.Fatalf("ResultSummary = %q, want %q", item.ResultSummary, "completed output")
	}
	if item.FailureReason != "" {
		t.Fatalf("FailureReason = %q, want empty", item.FailureReason)
	}
}

func TestRunCancelTransitions(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	item := newRun(t, now)

	assertTransition(t, item.Cancel(now.Add(time.Second)))
	assertStatus(t, item, StatusCancelled)

	err := item.StartRunning(now.Add(2 * time.Second))
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected invalid transition after cancel, got %v", err)
	}
}

func TestRunCancelEndsRunningSteps(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	item := newRun(t, now)
	if err := item.StartStep("prepare", "Preparing execution", now.Add(time.Second)); err != nil {
		t.Fatalf("start prepare step: %v", err)
	}
	if err := item.CompleteStep("prepare", "Prepared execution", now.Add(2*time.Second)); err != nil {
		t.Fatalf("complete prepare step: %v", err)
	}
	if err := item.StartStep("agent", "Agent execution started", now.Add(3*time.Second)); err != nil {
		t.Fatalf("start agent step: %v", err)
	}

	if err := item.Cancel(now.Add(4 * time.Second)); err != nil {
		t.Fatalf("cancel run: %v", err)
	}

	if item.Steps[0].Status != StatusCompleted {
		t.Fatalf("prepare step status = %s, want %s", item.Steps[0].Status, StatusCompleted)
	}
	agentStep := item.Steps[1]
	if agentStep.Status != StatusCancelled {
		t.Fatalf("agent step status = %s, want %s", agentStep.Status, StatusCancelled)
	}
	if agentStep.Message != "Run cancelled" {
		t.Fatalf("agent step message = %q, want Run cancelled", agentStep.Message)
	}
	if !agentStep.EndedAt.Equal(now.Add(4 * time.Second)) {
		t.Fatalf("agent step ended at = %v, want %v", agentStep.EndedAt, now.Add(4*time.Second))
	}
}

func TestRunCloneReturnsDetachedStepSlice(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	item := newRun(t, now)
	if err := item.StartStep("prepare", "Preparing execution", now.Add(time.Second)); err != nil {
		t.Fatalf("start step: %v", err)
	}

	clone := item.Clone()
	clone.Steps[0].Message = "changed"
	clone.Steps = append(clone.Steps, Step{Name: "agent"})

	if len(item.Steps) != 1 {
		t.Fatalf("len(original Steps) = %d, want 1", len(item.Steps))
	}
	if item.Steps[0].Message != "Preparing execution" {
		t.Fatalf("original step message = %q, want Preparing execution", item.Steps[0].Message)
	}
}

func TestTaskSpecValidation(t *testing.T) {
	tests := []struct {
		name string
		task TaskSpec
		want error
	}{
		{
			name: "valid",
			task: TaskSpec{Prompt: "do work"},
		},
		{
			name: "empty prompt",
			task: TaskSpec{Prompt: "   "},
			want: ErrEmptyPrompt,
		},
		{
			name: "prompt too long",
			task: TaskSpec{Prompt: strings.Repeat("a", MaxPromptLength+1)},
			want: ErrPromptTooLong,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.task.Validate()
			if !errors.Is(err, tt.want) {
				t.Fatalf("Validate() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func newRun(t *testing.T, now time.Time) *Run {
	t.Helper()

	item, err := New("run_test", TaskSpec{Prompt: "do work"}, now)
	if err != nil {
		t.Fatalf("new run: %v", err)
	}

	return item
}

func assertTransition(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatalf("transition failed: %v", err)
	}
}

func assertStatus(t *testing.T, item *Run, want Status) {
	t.Helper()

	if item.Status != want {
		t.Fatalf("status = %s, want %s", item.Status, want)
	}
}
