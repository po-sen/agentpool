package agent

import (
	"context"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

// Runner owns AgentPool's application-level agent behavior.
type Runner struct {
	model outbound.ModelClient
}

// RunnerOption configures a Runner.
type RunnerOption func(*Runner)

// RunRequest contains the application-level agent run input.
type RunRequest struct {
	RunID   run.RunID
	Task    run.TaskSpec
	Sandbox outbound.Sandbox
}

// RunResult contains the application-level agent run output.
type RunResult struct {
	Summary string
}

// NewRunner creates an application agent runner.
func NewRunner(model outbound.ModelClient, options ...RunnerOption) *Runner {
	runner := &Runner{
		model: model,
	}

	for _, option := range options {
		option(runner)
	}

	return runner
}

// Run executes the minimal application agent behavior for a run.
func (r *Runner) Run(ctx context.Context, request RunRequest) (RunResult, error) {
	response, err := r.model.Generate(ctx, outbound.ModelRequest{
		RunID: request.RunID,
		Messages: []outbound.ModelMessage{
			{Role: "user", Content: request.Task.Prompt},
		},
	})
	if err != nil {
		return RunResult{}, err
	}

	return RunResult{Summary: response.Content}, nil
}
