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

type runSession struct {
	request       RunRequest
	messages      []outbound.ModelMessage
	toolCallCount int
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

	session, err := r.newRunSession(ctx, request)
	if err != nil {
		return RunResult{}, err
	}

	for range r.maxTurns {
		result, done, err := r.runTurn(ctx, session)
		if err != nil {
			return RunResult{}, err
		}
		if done {
			return result, nil
		}
	}

	return RunResult{}, errors.New("agent reached max turns")
}

func (r *Runner) newRunSession(ctx context.Context, request RunRequest) (*runSession, error) {
	tools, err := r.tools.ListTools(ctx, outbound.ToolListRequest{
		RunID:   request.RunID,
		Sandbox: request.Sandbox,
	})
	if err != nil {
		return nil, err
	}

	return &runSession{
		request: request,
		messages: []outbound.ModelMessage{
			{Role: "system", Content: buildSystemPrompt(tools)},
			{Role: "user", Content: request.Task.Prompt},
		},
	}, nil
}

func (r *Runner) runTurn(ctx context.Context, session *runSession) (RunResult, bool, error) {
	response, err := r.model.Generate(ctx, outbound.ModelRequest{
		RunID:    session.request.RunID,
		Messages: session.messages,
	})
	if err != nil {
		return RunResult{}, false, err
	}

	parsed := parseAction(response.Content)
	switch parsed.status {
	case actionParseNaturalLanguage:
		return RunResult{Summary: response.Content, ToolCallCount: session.toolCallCount}, true, nil
	case actionParseProtocolError:
		session.messages = append(session.messages,
			outbound.ModelMessage{Role: "assistant", Content: response.Content},
			outbound.ModelMessage{Role: "user", Content: protocolCorrectionMessage()},
		)

		return RunResult{}, false, nil
	case actionParseValid:
		return r.handleAction(ctx, session, response.Content, parsed.action)
	default:
		return RunResult{}, false, errors.New("unknown agent action parse status")
	}
}

func (r *Runner) handleAction(ctx context.Context, session *runSession, content string, parsed action) (RunResult, bool, error) {
	switch parsed.Type {
	case actionTypeFinal:
		return finalResult(parsed.Summary, session.toolCallCount)
	case actionTypeToolCall:
		return r.handleToolCall(ctx, session, content, parsed)
	default:
		return RunResult{}, false, errors.New("unknown agent action type")
	}
}

func finalResult(summary string, toolCallCount int) (RunResult, bool, error) {
	if strings.TrimSpace(summary) == "" {
		return RunResult{}, false, errors.New("agent final summary is required")
	}

	return RunResult{Summary: summary, ToolCallCount: toolCallCount}, true, nil
}

func protocolCorrectionMessage() string {
	return `Protocol error:
Your previous response was JSON but did not match the AgentPool action protocol.
Return exactly one JSON object with either:
{"type":"final","summary":"..."}
or
{"type":"tool_call","tool":"<tool_name>","arguments":{"key":"value"}}
Do not return tool_result. Do not return multiple JSON objects. Do not use markdown fences.`
}

func (r *Runner) handleToolCall(ctx context.Context, session *runSession, content string, parsed action) (RunResult, bool, error) {
	session.messages = append(session.messages, outbound.ModelMessage{Role: "assistant", Content: content})
	if strings.TrimSpace(parsed.Tool) == "" {
		session.messages = append(session.messages, outbound.ModelMessage{
			Role:    "user",
			Content: "Tool error:\nmissing tool name",
		})
		return RunResult{}, false, nil
	}

	result, err := r.tools.RunTool(ctx, outbound.ToolCall{
		RunID:     session.request.RunID,
		Sandbox:   session.request.Sandbox,
		Name:      parsed.Tool,
		Arguments: parsed.Arguments,
	})
	if err != nil {
		return RunResult{}, false, err
	}
	session.toolCallCount++

	session.messages = append(session.messages, outbound.ModelMessage{
		Role:    "user",
		Content: toolObservation(parsed.Tool, result),
	})

	return RunResult{}, false, nil
}

func toolObservation(tool string, result outbound.ToolResult) string {
	if result.IsError {
		return fmt.Sprintf("Tool error for %s:\n%s", tool, result.Content)
	}

	return fmt.Sprintf("Tool result for %s:\n%s", tool, result.Content)
}
