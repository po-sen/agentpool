package agent

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

const defaultMaxTurns = 16

var errToolRunnerRequired = errors.New("tool runner is required")

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
	RunID            run.RunID
	Task             run.TaskSpec
	Context          outbound.ToolContext
	ProgressObserver ProgressObserver
}

// RunResult contains the application-level agent run output.
type RunResult struct {
	Summary       string
	ToolCallCount int
	ToolCalls     []ToolCallRecord
	AgentTurns    []TurnRecord
}

// ProgressObserver observes bounded agent progress while the run is still active.
type ProgressObserver func(context.Context, RunProgress) error

// RunProgress contains partial agent state observed during the agent loop.
type RunProgress struct {
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
	Index             int
	Status            string
	ActionType        string
	ToolName          string
	Message           string
	RequestMessages   []TurnMessageRecord
	RawResponse       string
	ResponseFormat    string
	ProtocolErrorCode string
	CorrectionMessage string
	ResponsePreview   string
	StartedAt         time.Time
	EndedAt           time.Time
}

// TurnMessageRecord captures one provider-facing model request message observed by the agent runner.
type TurnMessageRecord struct {
	Role       string
	Kind       string
	Content    string
	ToolCallID string
	ToolName   string
}

type runSession struct {
	request            RunRequest
	systemPrompt       string
	availableTools     map[string]outbound.ToolDefinition
	toolDefinitions    []outbound.ToolDefinition
	availableToolNames []string
	turns              []outbound.ModelTurn
	turnIndex          int
	toolCallCount      int
	toolCalls          []ToolCallRecord
	turnRecords        []TurnRecord
	currentTurn        *TurnRecord
	progressObserver   ProgressObserver
	progressErr        error
	ctx                context.Context
}

type modelTurnContext struct {
	content         string
	requestMessages []TurnMessageRecord
	responseFormat  string
	index           int
	startedAt       time.Time
	endedAt         time.Time
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
		if session.progressErr != nil {
			return session.partialResult(), session.progressErr
		}
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
	if session.progressErr != nil {
		return session.partialResult(), session.progressErr
	}

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

	systemPrompt := buildSystemPrompt(tools)
	return &runSession{
		request:            request,
		systemPrompt:       systemPrompt,
		availableTools:     availableToolMap(tools),
		toolDefinitions:    copyToolDefinitions(tools),
		availableToolNames: availableToolNames(tools),
		turns:              buildInitialTurns(request.Task),
		progressObserver:   request.ProgressObserver,
		ctx:                ctx,
	}, nil
}

func (r *Runner) runTurn(ctx context.Context, session *runSession) (RunResult, bool, error) {
	turnIndex := session.nextTurnIndex()
	startedAt := r.clock()
	fallbackRequestMessages := copyModelRequestMessages(session.systemPrompt, session.turns)
	session.startTurnProgress(TurnRecord{
		Index:           turnIndex,
		Status:          run.AgentTurnStatusModelResponse,
		Message:         "waiting for model response",
		RequestMessages: fallbackRequestMessages,
		StartedAt:       startedAt,
	})
	if session.progressErr != nil {
		return session.partialResult(), false, session.progressErr
	}
	response, err := r.model.Generate(ctx, outbound.ModelRequest{
		RunID:        session.request.RunID,
		Instructions: session.systemPrompt,
		Turns:        session.turns,
		Tools:        session.toolDefinitions,
	})
	endedAt := r.clock()
	requestMessages := responseRequestMessages(response, fallbackRequestMessages)
	if err != nil {
		session.recordTurn(TurnRecord{
			Index:           turnIndex,
			Status:          run.AgentTurnStatusModelError,
			Message:         messageModelGenerateFailed,
			RequestMessages: requestMessages,
			StartedAt:       startedAt,
			EndedAt:         endedAt,
		})

		return session.partialResult(), false, newAgentError(ErrorCodeModelGenerateFailed, "", err)
	}

	responseFormat := classifyModelResponseFormat(response.Content)
	if len(response.ToolCalls) > 0 {
		rawResponse := nativeToolCallsRawResponse(response.ToolCalls)
		turn := modelTurnContext{
			content:         rawResponse,
			requestMessages: requestMessages,
			responseFormat:  classifyModelResponseFormat(rawResponse),
			index:           turnIndex,
			startedAt:       startedAt,
			endedAt:         endedAt,
		}

		return r.handleNativeToolCalls(ctx, session, turn, response.ToolCalls)
	}

	parsed := parseAction(response.Content)
	switch parsed.status {
	case actionParseProtocolError:
		message := "model response did not match AgentPool action protocol"
		if parsed.parseErr.Message != "" {
			message = parsed.parseErr.Message
		}
		correctionMessage := buildProtocolCorrectionMessage(parsed.parseErr)
		session.recordTurn(TurnRecord{
			Index:             turnIndex,
			Status:            run.AgentTurnStatusProtocolError,
			Message:           message,
			RequestMessages:   requestMessages,
			RawResponse:       response.Content,
			ResponseFormat:    responseFormat,
			ProtocolErrorCode: parsed.parseErr.Code,
			CorrectionMessage: correctionMessage,
			ResponsePreview:   previewModelResponse(response.Content),
			StartedAt:         startedAt,
			EndedAt:           endedAt,
		})
		session.turns = append(session.turns,
			assistantAttemptTurn(response.Content),
			runtimeTurn(outbound.ModelPartKindProtocolCorrection, correctionMessage),
		)

		return RunResult{}, false, nil
	case actionParseValid:
		turn := modelTurnContext{
			content:         response.Content,
			requestMessages: requestMessages,
			responseFormat:  responseFormat,
			index:           turnIndex,
			startedAt:       startedAt,
			endedAt:         endedAt,
		}

		return r.handleAction(ctx, session, turn, parsed.action)
	default:
		session.recordTurn(TurnRecord{
			Index:           turnIndex,
			Status:          run.AgentTurnStatusProtocolError,
			Message:         messageAgentProtocolError,
			RequestMessages: requestMessages,
			RawResponse:     response.Content,
			ResponseFormat:  responseFormat,
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
	turn modelTurnContext,
	parsed action,
) (RunResult, bool, error) {
	switch parsed.Type {
	case actionTypeFinal:
		if err := validateFinalSummary(parsed.Summary); err != nil {
			session.recordTurn(TurnRecord{
				Index:           turn.index,
				Status:          run.AgentTurnStatusFinal,
				ActionType:      run.AgentTurnActionTypeFinal,
				Message:         messageFinalSummaryInvalid,
				RequestMessages: turn.requestMessages,
				RawResponse:     turn.content,
				ResponseFormat:  turn.responseFormat,
				ResponsePreview: previewModelResponse(turn.content),
				StartedAt:       turn.startedAt,
				EndedAt:         turn.endedAt,
			})

			return session.partialResult(), false, newAgentError(ErrorCodeFinalSummaryInvalid, "", err)
		}
		session.recordTurn(TurnRecord{
			Index:           turn.index,
			Status:          run.AgentTurnStatusFinal,
			ActionType:      run.AgentTurnActionTypeFinal,
			Message:         "model returned final answer",
			RequestMessages: turn.requestMessages,
			RawResponse:     turn.content,
			ResponseFormat:  turn.responseFormat,
			ResponsePreview: previewModelResponse(parsed.Summary),
			StartedAt:       turn.startedAt,
			EndedAt:         turn.endedAt,
		})

		return session.finalResult(parsed.Summary), true, nil
	case actionTypeToolCall:
		if !session.toolIsAvailable(parsed.Tool) {
			return handleUnavailableToolCall(session, turn, parsed)
		}
		if placeholders := placeholderArgumentValues(parsed.Arguments); len(placeholders) > 0 {
			return handlePlaceholderToolCall(session, turn, parsed, placeholders)
		}

		session.recordTurn(TurnRecord{
			Index:           turn.index,
			Status:          run.AgentTurnStatusToolCall,
			ActionType:      run.AgentTurnActionTypeToolCall,
			ToolName:        parsed.Tool,
			Message:         "model requested tool call",
			RequestMessages: turn.requestMessages,
			RawResponse:     turn.content,
			ResponseFormat:  turn.responseFormat,
			ResponsePreview: previewModelResponse(turn.content),
			StartedAt:       turn.startedAt,
			EndedAt:         turn.endedAt,
		})

		return r.handleToolCall(ctx, session, turn.content, parsed)
	default:
		session.recordTurn(TurnRecord{
			Index:           turn.index,
			Status:          run.AgentTurnStatusProtocolError,
			Message:         messageAgentProtocolError,
			RequestMessages: turn.requestMessages,
			RawResponse:     turn.content,
			ResponseFormat:  turn.responseFormat,
			ResponsePreview: previewModelResponse(turn.content),
			StartedAt:       turn.startedAt,
			EndedAt:         turn.endedAt,
		})

		return session.partialResult(), false, newAgentError(
			ErrorCodeAgentProtocolError,
			"",
			errors.New("unknown agent action type"),
		)
	}
}

func validateFinalSummary(summary string) error {
	if strings.TrimSpace(summary) == "" {
		return errors.New("agent final summary is required")
	}

	return nil
}
