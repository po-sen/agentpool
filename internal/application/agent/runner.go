package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

const defaultMaxTurns = 4

var errToolRunnerRequired = errors.New("tool runner is required")

const agentTurnPreviewTruncatedMarker = "\n... [truncated]"

// Runner owns AgentPool's application-level agent behavior.
type Runner struct {
	model    outbound.ModelClient
	tools    outbound.ToolRunner
	maxTurns int
	clock    func() time.Time
}

// RunnerOption configures a Runner.
type RunnerOption func(*Runner)

// RunRequest contains the application-level agent run input.
type RunRequest struct {
	RunID   run.RunID
	Task    run.TaskSpec
	Context outbound.ToolContext
}

// RunResult contains the application-level agent run output.
type RunResult struct {
	Summary       string
	ToolCallCount int
	ToolCalls     []ToolCallRecord
	AgentTurns    []TurnRecord
}

// ToolCallRecord captures one tool execution observed by the agent loop.
type ToolCallRecord struct {
	Name      string
	Arguments map[string]string
	Result    string
	IsError   bool
	StartedAt time.Time
	EndedAt   time.Time
}

// TurnRecord captures one model-loop diagnostic observed by the agent runner.
type TurnRecord struct {
	Index           int
	Status          string
	ActionType      string
	ToolName        string
	Message         string
	ResponsePreview string
	StartedAt       time.Time
	EndedAt         time.Time
}

type runSession struct {
	request       RunRequest
	messages      []outbound.ModelMessage
	turnIndex     int
	toolCallCount int
	toolCalls     []ToolCallRecord
	turns         []TurnRecord
}

// NewRunner creates an application agent runner.
func NewRunner(model outbound.ModelClient, tools outbound.ToolRunner, options ...RunnerOption) *Runner {
	runner := &Runner{
		model:    model,
		tools:    tools,
		maxTurns: defaultMaxTurns,
		clock: func() time.Time {
			return time.Now().UTC()
		},
	}

	for _, option := range options {
		option(runner)
	}

	return runner
}

// WithClock injects a time source for tests.
func WithClock(clock func() time.Time) RunnerOption {
	return func(runner *Runner) {
		if clock != nil {
			runner.clock = clock
		}
	}
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
		return RunResult{}, newAgentError(
			ErrorCodeUnknown,
			errToolRunnerRequired.Error(),
			errToolRunnerRequired,
		)
	}

	session, err := r.newRunSession(ctx, request)
	if err != nil {
		return RunResult{}, err
	}

	for range r.maxTurns {
		result, done, err := r.runTurn(ctx, session)
		if err != nil {
			return result, err
		}
		if done {
			return result, nil
		}
	}

	startedAt := r.clock()
	endedAt := r.clock()
	session.recordTurn(TurnRecord{
		Index:     session.nextTurnIndex(),
		Status:    run.AgentTurnStatusMaxTurns,
		Message:   messageAgentMaxTurns,
		StartedAt: startedAt,
		EndedAt:   endedAt,
	})

	return session.partialResult(), newAgentError(ErrorCodeAgentMaxTurns, "", nil)
}

func (r *Runner) newRunSession(ctx context.Context, request RunRequest) (*runSession, error) {
	tools, err := r.tools.ListTools(ctx, outbound.ToolListRequest{
		RunID:   request.RunID,
		Context: request.Context,
	})
	if err != nil {
		return nil, newAgentError(ErrorCodeToolListFailed, "", err)
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
	turnIndex := session.nextTurnIndex()
	startedAt := r.clock()
	response, err := r.model.Generate(ctx, outbound.ModelRequest{
		RunID:    session.request.RunID,
		Messages: session.messages,
	})
	endedAt := r.clock()
	if err != nil {
		session.recordTurn(TurnRecord{
			Index:     turnIndex,
			Status:    run.AgentTurnStatusModelError,
			Message:   messageModelGenerateFailed,
			StartedAt: startedAt,
			EndedAt:   endedAt,
		})

		return session.partialResult(), false, newAgentError(ErrorCodeModelGenerateFailed, "", err)
	}

	parsed := parseAction(response.Content)
	switch parsed.status {
	case actionParseNaturalLanguage:
		if err := validateFinalSummary(response.Content); err != nil {
			session.recordTurn(TurnRecord{
				Index:           turnIndex,
				Status:          run.AgentTurnStatusFinal,
				ActionType:      run.AgentTurnActionTypeFinal,
				Message:         messageFinalSummaryInvalid,
				ResponsePreview: previewModelResponse(response.Content),
				StartedAt:       startedAt,
				EndedAt:         endedAt,
			})

			return session.partialResult(), false, newAgentError(ErrorCodeFinalSummaryInvalid, "", err)
		}
		session.recordTurn(TurnRecord{
			Index:           turnIndex,
			Status:          run.AgentTurnStatusNaturalLanguageFinal,
			Message:         "model returned natural-language final answer",
			ResponsePreview: previewModelResponse(response.Content),
			StartedAt:       startedAt,
			EndedAt:         endedAt,
		})

		return session.finalResult(response.Content), true, nil
	case actionParseProtocolError:
		session.recordTurn(TurnRecord{
			Index:           turnIndex,
			Status:          run.AgentTurnStatusProtocolError,
			Message:         "model response did not match AgentPool action protocol",
			ResponsePreview: previewModelResponse(response.Content),
			StartedAt:       startedAt,
			EndedAt:         endedAt,
		})
		session.messages = append(session.messages,
			outbound.ModelMessage{Role: "assistant", Content: response.Content},
			outbound.ModelMessage{Role: "user", Content: protocolCorrectionMessage()},
		)

		return RunResult{}, false, nil
	case actionParseValid:
		return r.handleAction(ctx, session, response.Content, parsed.action, turnIndex, startedAt, endedAt)
	default:
		session.recordTurn(TurnRecord{
			Index:           turnIndex,
			Status:          run.AgentTurnStatusProtocolError,
			Message:         messageAgentProtocolError,
			ResponsePreview: previewModelResponse(response.Content),
			StartedAt:       startedAt,
			EndedAt:         endedAt,
		})

		return session.partialResult(), false, newAgentError(
			ErrorCodeAgentProtocolError,
			"",
			errors.New("unknown agent action parse status"),
		)
	}
}

func (r *Runner) handleAction(
	ctx context.Context,
	session *runSession,
	content string,
	parsed action,
	turnIndex int,
	startedAt time.Time,
	endedAt time.Time,
) (RunResult, bool, error) {
	switch parsed.Type {
	case actionTypeFinal:
		if err := validateFinalSummary(parsed.Summary); err != nil {
			session.recordTurn(TurnRecord{
				Index:           turnIndex,
				Status:          run.AgentTurnStatusFinal,
				ActionType:      run.AgentTurnActionTypeFinal,
				Message:         messageFinalSummaryInvalid,
				ResponsePreview: previewModelResponse(content),
				StartedAt:       startedAt,
				EndedAt:         endedAt,
			})

			return session.partialResult(), false, newAgentError(ErrorCodeFinalSummaryInvalid, "", err)
		}
		session.recordTurn(TurnRecord{
			Index:           turnIndex,
			Status:          run.AgentTurnStatusFinal,
			ActionType:      run.AgentTurnActionTypeFinal,
			Message:         "model returned final answer",
			ResponsePreview: previewModelResponse(parsed.Summary),
			StartedAt:       startedAt,
			EndedAt:         endedAt,
		})

		return session.finalResult(parsed.Summary), true, nil
	case actionTypeToolCall:
		session.recordTurn(TurnRecord{
			Index:           turnIndex,
			Status:          run.AgentTurnStatusToolCall,
			ActionType:      run.AgentTurnActionTypeToolCall,
			ToolName:        parsed.Tool,
			Message:         "model requested tool call",
			ResponsePreview: previewModelResponse(content),
			StartedAt:       startedAt,
			EndedAt:         endedAt,
		})

		return r.handleToolCall(ctx, session, content, parsed)
	default:
		session.recordTurn(TurnRecord{
			Index:           turnIndex,
			Status:          run.AgentTurnStatusProtocolError,
			Message:         messageAgentProtocolError,
			ResponsePreview: previewModelResponse(content),
			StartedAt:       startedAt,
			EndedAt:         endedAt,
		})

		return session.partialResult(), false, newAgentError(
			ErrorCodeAgentProtocolError,
			"",
			errors.New("unknown agent action type"),
		)
	}
}

func (s *runSession) finalResult(summary string) RunResult {
	return RunResult{
		Summary:       summary,
		ToolCallCount: s.toolCallCount,
		ToolCalls:     copyToolCallRecords(s.toolCalls),
		AgentTurns:    copyTurnRecords(s.turns),
	}
}

func (s *runSession) partialResult() RunResult {
	return RunResult{
		ToolCallCount: s.toolCallCount,
		ToolCalls:     copyToolCallRecords(s.toolCalls),
		AgentTurns:    copyTurnRecords(s.turns),
	}
}

func validateFinalSummary(summary string) error {
	if strings.TrimSpace(summary) == "" {
		return errors.New("agent final summary is required")
	}

	return nil
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

	startedAt := r.clock()
	call := outbound.ToolCall{
		RunID:     session.request.RunID,
		Context:   session.request.Context,
		Name:      parsed.Tool,
		Arguments: parsed.Arguments,
	}
	result, err := r.tools.RunTool(ctx, call)
	endedAt := r.clock()
	if err != nil {
		session.recordToolCall(parsed.Tool, parsed.Arguments, "tool execution failed", true, startedAt, endedAt)

		return session.partialResult(), false, newAgentError(ErrorCodeToolExecutionFailed, "", err)
	}
	session.toolCallCount++
	session.recordToolCall(parsed.Tool, parsed.Arguments, result.Content, result.IsError, startedAt, endedAt)

	session.messages = append(session.messages, outbound.ModelMessage{
		Role:    "user",
		Content: toolObservation(parsed.Tool, result),
	})

	return RunResult{}, false, nil
}

func (s *runSession) recordToolCall(
	name string,
	arguments map[string]string,
	result string,
	isError bool,
	startedAt time.Time,
	endedAt time.Time,
) {
	s.toolCalls = append(s.toolCalls, ToolCallRecord{
		Name:      name,
		Arguments: copyArguments(arguments),
		Result:    result,
		IsError:   isError,
		StartedAt: startedAt,
		EndedAt:   endedAt,
	})
}

func (s *runSession) nextTurnIndex() int {
	s.turnIndex++

	return s.turnIndex
}

func (s *runSession) recordTurn(record TurnRecord) {
	record.ResponsePreview = previewModelResponse(record.ResponsePreview)
	s.turns = append(s.turns, record)
}

func copyToolCallRecords(records []ToolCallRecord) []ToolCallRecord {
	if len(records) == 0 {
		return nil
	}

	copied := make([]ToolCallRecord, 0, len(records))
	for _, record := range records {
		item := record
		item.Arguments = copyArguments(record.Arguments)
		copied = append(copied, item)
	}

	return copied
}

func copyTurnRecords(records []TurnRecord) []TurnRecord {
	if len(records) == 0 {
		return nil
	}

	copied := make([]TurnRecord, 0, len(records))
	for _, record := range records {
		item := record
		item.ResponsePreview = previewModelResponse(record.ResponsePreview)
		copied = append(copied, item)
	}

	return copied
}

func copyArguments(arguments map[string]string) map[string]string {
	if len(arguments) == 0 {
		return nil
	}

	copied := make(map[string]string, len(arguments))
	for key, value := range arguments {
		copied[key] = value
	}

	return copied
}

func toolObservation(tool string, result outbound.ToolResult) string {
	if result.IsError {
		return fmt.Sprintf("Tool error for %s:\n%s", tool, result.Content)
	}

	return fmt.Sprintf("Tool result for %s:\n%s", tool, result.Content)
}

func previewModelResponse(content string) string {
	content = strings.ToValidUTF8(content, "\uFFFD")
	if len(content) <= run.MaxAgentTurnPreviewLength {
		return content
	}

	maxContentLength := run.MaxAgentTurnPreviewLength - len(agentTurnPreviewTruncatedMarker)
	if maxContentLength < 0 {
		maxContentLength = 0
	}

	for maxContentLength > 0 && !utf8.ValidString(content[:maxContentLength]) {
		maxContentLength--
	}

	return content[:maxContentLength] + agentTurnPreviewTruncatedMarker
}
