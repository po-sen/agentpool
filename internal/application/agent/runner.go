package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

const defaultMaxTurns = 4

// Runner owns AgentPool's application-level agent behavior.
type Runner struct {
	model    outbound.ModelClient
	tools    outbound.ToolRunner
	maxTurns int
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
	Summary       string
	ToolCallCount int
}

// NewRunner creates an application agent runner.
func NewRunner(model outbound.ModelClient, tools outbound.ToolRunner, options ...RunnerOption) *Runner {
	runner := &Runner{
		model:    model,
		tools:    tools,
		maxTurns: defaultMaxTurns,
	}

	for _, option := range options {
		option(runner)
	}

	return runner
}

// WithMaxTurns configures the maximum number of model turns per run.
func WithMaxTurns(maxTurns int) RunnerOption {
	return func(runner *Runner) {
		if maxTurns > 0 {
			runner.maxTurns = maxTurns
		}
	}
}

// Run executes the minimal application agent behavior for a run.
func (r *Runner) Run(ctx context.Context, request RunRequest) (RunResult, error) {
	if r.tools == nil {
		return RunResult{}, errors.New("tool runner is required")
	}

	tools, err := r.tools.ListTools(ctx, outbound.ToolListRequest{
		RunID:   request.RunID,
		Sandbox: request.Sandbox,
	})
	if err != nil {
		return RunResult{}, err
	}

	messages := []outbound.ModelMessage{
		{Role: "system", Content: buildSystemPrompt(tools)},
		{Role: "user", Content: request.Task.Prompt},
	}
	toolCallCount := 0

	for range r.maxTurns {
		response, err := r.model.Generate(ctx, outbound.ModelRequest{
			RunID:    request.RunID,
			Messages: messages,
		})
		if err != nil {
			return RunResult{}, err
		}

		parsed, ok := parseAction(response.Content)
		if !ok {
			return RunResult{Summary: response.Content, ToolCallCount: toolCallCount}, nil
		}

		switch parsed.Type {
		case actionTypeFinal:
			if strings.TrimSpace(parsed.Summary) == "" {
				return RunResult{}, errors.New("agent final summary is required")
			}

			return RunResult{Summary: parsed.Summary, ToolCallCount: toolCallCount}, nil
		case actionTypeToolCall:
			messages = append(messages, outbound.ModelMessage{Role: "assistant", Content: response.Content})
			if strings.TrimSpace(parsed.Tool) == "" {
				messages = append(messages, outbound.ModelMessage{
					Role:    "user",
					Content: "Tool error:\nmissing tool name",
				})
				continue
			}

			result, err := r.tools.RunTool(ctx, outbound.ToolCall{
				RunID:     request.RunID,
				Sandbox:   request.Sandbox,
				Name:      parsed.Tool,
				Arguments: parsed.Arguments,
			})
			if err != nil {
				return RunResult{}, err
			}
			toolCallCount++

			messages = append(messages, outbound.ModelMessage{
				Role:    "user",
				Content: toolObservation(parsed.Tool, result),
			})
		default:
			return RunResult{}, errors.New("unknown agent action type")
		}
	}

	return RunResult{}, errors.New("agent reached max turns")
}

func toolObservation(tool string, result outbound.ToolResult) string {
	if result.IsError {
		return fmt.Sprintf("Tool error for %s:\n%s", tool, result.Content)
	}

	return fmt.Sprintf("Tool result for %s:\n%s", tool, result.Content)
}
