package run

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

const (
	// MaxAgentSystemPromptLength bounds the stored agent system prompt kept for protected diagnostics.
	MaxAgentSystemPromptLength = 16 << 10
)

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
	ID                        RunID
	Task                      TaskSpec
	Status                    Status
	ResultSummary             string
	FailureReason             string
	FailureCode               string
	FailureMessage            string
	Steps                     []Step
	ToolCalls                 []ToolCall
	AgentTurns                []AgentTurn
	Artifacts                 []Artifact
	AgentSystemPrompt         string
	AgentPromptVersion        string
	AgentPromptSHA256         string
	AgentSystemPromptRedacted bool
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
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
		Task:      task.Clone(),
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
	clone.Task = r.Task.Clone()
	if len(r.Steps) > 0 {
		clone.Steps = append([]Step(nil), r.Steps...)
	}
	clone.ToolCalls = copyToolCalls(r.ToolCalls)
	clone.AgentTurns = copyAgentTurns(r.AgentTurns)
	clone.Artifacts = copyArtifacts(r.Artifacts)

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

	if err := r.TransitionTo(StatusCancelled, now); err != nil {
		return err
	}

	r.cancelRunningSteps(now)

	return nil
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

// CompleteWithResult marks the run as successfully completed and stores the agent output summary.
func (r *Run) CompleteWithResult(now time.Time, summary string) error {
	if err := r.TransitionTo(StatusCompleted, now); err != nil {
		return err
	}

	r.ResultSummary = summary
	r.FailureReason = ""
	r.FailureCode = ""
	r.FailureMessage = ""

	return nil
}

// RecordToolCalls replaces the stored tool call history with detached bounded records.
func (r *Run) RecordToolCalls(now time.Time, calls []ToolCall) {
	r.ToolCalls = copyToolCalls(calls)
	r.UpdatedAt = now
}

// RecordAgentTurns replaces the stored model-loop diagnostics with detached bounded records.
func (r *Run) RecordAgentTurns(now time.Time, turns []AgentTurn) {
	r.AgentTurns = copyAgentTurns(turns)
	r.UpdatedAt = now
}

// RecordAgentSystemPrompt stores protected prompt diagnostics for the agent prompt used by the run.
func (r *Run) RecordAgentSystemPrompt(now time.Time, prompt string, version string) {
	r.AgentSystemPrompt = truncateUTF8Text(prompt, MaxAgentSystemPromptLength)
	r.AgentPromptVersion = strings.TrimSpace(version)
	if prompt != "" {
		sum := sha256.Sum256([]byte(prompt))
		r.AgentPromptSHA256 = hex.EncodeToString(sum[:])
		r.AgentSystemPromptRedacted = true
	} else {
		r.AgentPromptSHA256 = ""
		r.AgentSystemPromptRedacted = false
	}
	r.UpdatedAt = now
}

// RecordArtifacts replaces stored run artifacts with detached bounded records.
func (r *Run) RecordArtifacts(now time.Time, artifacts []Artifact) {
	r.Artifacts = copyArtifacts(artifacts)
	r.UpdatedAt = now
}

// ArtifactByPath returns one detached artifact by its workspace-relative path.
func (r *Run) ArtifactByPath(path string) (Artifact, bool) {
	if r == nil {
		return Artifact{}, false
	}
	for _, artifact := range r.Artifacts {
		if artifact.Path == path {
			return artifact.Clone(), true
		}
	}

	return Artifact{}, false
}

// Fail marks the run as failed.
func (r *Run) Fail(now time.Time) error {
	return r.TransitionTo(StatusFailed, now)
}

// FailWithReason marks the run as failed and stores the failure reason.
func (r *Run) FailWithReason(now time.Time, reason string) error {
	return r.FailWithDiagnostics(now, reason, "", "")
}

// FailWithDiagnostics marks the run as failed and stores safe failure diagnostics.
func (r *Run) FailWithDiagnostics(now time.Time, reason string, code string, message string) error {
	if err := r.TransitionTo(StatusFailed, now); err != nil {
		return err
	}

	r.ResultSummary = ""
	r.FailureReason = reason
	r.FailureCode = code
	r.FailureMessage = message

	return nil
}
