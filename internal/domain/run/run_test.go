package run_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestRunStatusTransitions(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	item := newRun(t, now)

	assertTransition(t, item.StartPreparing(now.Add(time.Second)))
	assertStatus(t, item, run.StatusPreparing)

	assertTransition(t, item.StartRunning(now.Add(2*time.Second)))
	assertStatus(t, item, run.StatusRunning)

	assertTransition(t, item.WaitForApproval(now.Add(3*time.Second)))
	assertStatus(t, item, run.StatusWaitingApproval)

	assertTransition(t, item.StartRunning(now.Add(4*time.Second)))
	assertStatus(t, item, run.StatusRunning)

	assertTransition(t, item.Complete(now.Add(5*time.Second)))
	assertStatus(t, item, run.StatusCompleted)
}

func TestRunRejectsInvalidTransition(t *testing.T) {
	item := newRun(t, time.Unix(100, 0).UTC())

	err := item.Complete(time.Unix(101, 0).UTC())
	if !errors.Is(err, run.ErrInvalidTransition) {
		t.Fatalf("expected invalid transition error, got %v", err)
	}
	assertStatus(t, item, run.StatusQueued)
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

	assertStatus(t, item, run.StatusCompleted)
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

	assertStatus(t, item, run.StatusFailed)
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
	if !errors.Is(err, run.ErrInvalidTransition) {
		t.Fatalf("CompleteWithResult() error = %v, want %v", err, run.ErrInvalidTransition)
	}
	assertStatus(t, item, run.StatusQueued)
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
	if !errors.Is(err, run.ErrInvalidTransition) {
		t.Fatalf("FailWithReason() error = %v, want %v", err, run.ErrInvalidTransition)
	}
	assertStatus(t, item, run.StatusCompleted)
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
	assertStatus(t, item, run.StatusCancelled)

	err := item.StartRunning(now.Add(2 * time.Second))
	if !errors.Is(err, run.ErrInvalidTransition) {
		t.Fatalf("expected invalid transition after cancel, got %v", err)
	}
}

func TestTaskSpecValidation(t *testing.T) {
	tests := []struct {
		name string
		task run.TaskSpec
		want error
	}{
		{
			name: "valid",
			task: run.TaskSpec{Prompt: "do work"},
		},
		{
			name: "empty prompt",
			task: run.TaskSpec{Prompt: "   "},
			want: run.ErrEmptyPrompt,
		},
		{
			name: "prompt too long",
			task: run.TaskSpec{Prompt: strings.Repeat("a", run.MaxPromptLength+1)},
			want: run.ErrPromptTooLong,
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

func newRun(t *testing.T, now time.Time) *run.Run {
	t.Helper()

	item, err := run.New("run_test", run.TaskSpec{Prompt: "do work"}, now)
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

func assertStatus(t *testing.T, item *run.Run, want run.Status) {
	t.Helper()

	if item.Status != want {
		t.Fatalf("status = %s, want %s", item.Status, want)
	}
}
