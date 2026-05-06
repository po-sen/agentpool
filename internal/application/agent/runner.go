package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

const defaultMaxTurns = 16

var errToolRunnerRequired = errors.New("tool runner is required")

const toolNameSandboxExec = "sandbox_exec"

const toolArgumentCommand = "command"

const agentTurnPreviewTruncatedMarker = "\n... [truncated]"

const (
	modelResponseFormatEmpty              = "empty"
	modelResponseFormatPlainText          = "plain_text"
	modelResponseFormatMarkdownFence      = "markdown_fence"
	modelResponseFormatJSONObject         = "json_object"
	modelResponseFormatJSONArray          = "json_array"
	modelResponseFormatJSONScalar         = "json_scalar"
	modelResponseFormatInvalidJSON        = "invalid_json"
	modelResponseFormatMultipleJSONValues = "multiple_json_values"
)

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
	request                          RunRequest
	systemPrompt                     string
	availableTools                   map[string]outbound.ToolDefinition
	toolDefinitions                  []outbound.ToolDefinition
	availableToolNames               []string
	turns                            []outbound.ModelTurn
	turnIndex                        int
	toolCallCount                    int
	toolCalls                        []ToolCallRecord
	turnRecords                      []TurnRecord
	sandboxExecErrored               bool
	lastErroredSandboxExecCommand    string
	lastSuccessfulSandboxExecCommand string
	sandboxExecRetryRequired         bool
	sandboxExecRetryMessage          string
	sandboxExecRetryCorrection       string
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

	systemPrompt := buildSystemPrompt(tools)
	return &runSession{
		request:            request,
		systemPrompt:       systemPrompt,
		availableTools:     availableToolMap(tools),
		toolDefinitions:    copyToolDefinitions(tools),
		availableToolNames: availableToolNames(tools),
		turns:              buildInitialTurns(request.Task),
	}, nil
}

func (r *Runner) runTurn(ctx context.Context, session *runSession) (RunResult, bool, error) {
	turnIndex := session.nextTurnIndex()
	startedAt := r.clock()
	fallbackRequestMessages := copyModelRequestMessages(session.systemPrompt, session.turns)
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
		if session.sandboxExecErrored || session.sandboxExecRetryRequired {
			message := "final answer attempted after failed sandbox_exec"
			correctionMessage := buildSandboxExecErrorFinalCorrectionMessage()
			if session.sandboxExecRetryRequired {
				message = session.sandboxExecRetryMessage
				if message == "" {
					message = "final answer attempted after invalid sandbox_exec"
				}
				correctionMessage = session.sandboxExecRetryCorrection
				if correctionMessage == "" {
					correctionMessage = buildSandboxExecStaticOutputCorrectionMessage()
				}
			}
			session.recordTurn(TurnRecord{
				Index:             turn.index,
				Status:            run.AgentTurnStatusProtocolError,
				ActionType:        run.AgentTurnActionTypeFinal,
				Message:           message,
				RequestMessages:   turn.requestMessages,
				RawResponse:       turn.content,
				ResponseFormat:    turn.responseFormat,
				CorrectionMessage: correctionMessage,
				ResponsePreview:   previewModelResponse(parsed.Summary),
				StartedAt:         turn.startedAt,
				EndedAt:           turn.endedAt,
			})
			session.turns = append(session.turns,
				assistantAttemptTurn(turn.content),
				runtimeTurn(outbound.ModelPartKindToolCorrection, correctionMessage),
			)

			return RunResult{}, false, nil
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
		if sandboxExecCommandOnlyPrintsStaticText(parsed.Tool, parsed.Arguments) {
			return handleStaticOutputSandboxExecToolCall(session, turn, parsed.Tool, turn.content)
		}
		if sandboxExecCommandUsesUnverifiedNumericalSolve(parsed.Tool, parsed.Arguments) {
			return handleUnverifiedNumericalSandboxExecToolCall(session, turn, parsed.Tool, turn.content)
		}
		if sandboxExecCommandWritesReadOnlyPDFTextOutput(parsed.Tool, parsed.Arguments) {
			return handleReadOnlyPDFTextOutputSandboxExecToolCall(session, turn, parsed.Tool, turn.content)
		}
		if sandboxExecCommandRepeatsFailedCommand(session, parsed.Tool, parsed.Arguments) {
			return handleRepeatedFailedSandboxExecToolCall(session, turn, parsed.Tool, turn.content)
		}
		if sandboxExecCommandRepeatsSuccessfulCommand(session, parsed.Tool, parsed.Arguments) {
			return handleRepeatedSuccessfulSandboxExecToolCall(session, turn, parsed.Tool, turn.content)
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

func buildInitialTurns(task run.TaskSpec) []outbound.ModelTurn {
	parts := []outbound.ModelPart{
		{Kind: outbound.ModelPartKindTaskPrompt, Text: task.Prompt},
	}
	if workspaceContext := buildWorkspaceContextPart(task); workspaceContext != "" {
		parts = append(parts, outbound.ModelPart{
			Kind: outbound.ModelPartKindWorkspaceContext,
			Text: workspaceContext,
		})
	}

	return []outbound.ModelTurn{{Role: outbound.ModelRoleUser, Parts: parts}}
}

func assistantTurn(content string) outbound.ModelTurn {
	return outbound.ModelTurn{
		Role: outbound.ModelRoleAssistant,
		Parts: []outbound.ModelPart{
			{Kind: outbound.ModelPartKindAssistantResponse, Text: content},
		},
	}
}

func assistantAttemptTurn(content string) outbound.ModelTurn {
	return outbound.ModelTurn{
		Role: outbound.ModelRoleAssistant,
		Parts: []outbound.ModelPart{
			{Kind: outbound.ModelPartKindAssistantAttempt, Text: boundedAssistantAttempt(content)},
		},
	}
}

func assistantToolCallTurn(content string, calls []outbound.ModelToolCall) outbound.ModelTurn {
	parts := []outbound.ModelPart{}
	if text := strings.TrimSpace(content); text != "" {
		parts = append(parts, outbound.ModelPart{
			Kind: outbound.ModelPartKindAssistantResponse,
			Text: text,
		})
	}
	for _, call := range calls {
		parts = append(parts, outbound.ModelPart{
			Kind:          outbound.ModelPartKindToolCall,
			ToolCallID:    call.ID,
			ToolName:      call.Name,
			ToolArguments: copyArguments(call.Arguments),
		})
	}

	return outbound.ModelTurn{
		Role:  outbound.ModelRoleAssistant,
		Parts: parts,
	}
}

func runtimeTurn(kind outbound.ModelPartKind, content string) outbound.ModelTurn {
	return outbound.ModelTurn{
		Role: outbound.ModelRoleRuntime,
		Parts: []outbound.ModelPart{
			{Kind: kind, Text: content},
		},
	}
}

func toolResultTurn(results []outbound.ModelPart) outbound.ModelTurn {
	return outbound.ModelTurn{
		Role:  outbound.ModelRoleTool,
		Parts: results,
	}
}

func buildWorkspaceContextPart(task run.TaskSpec) string {
	if len(task.Attachments) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("Workspace input files available to tools:\n")
	for _, attachment := range task.Attachments {
		builder.WriteString("- path: ")
		builder.WriteString(attachment.Filename)
		builder.WriteString("; virtual_path: /workspace/input/")
		builder.WriteString(attachment.Filename)
		if attachment.MediaType != "" {
			builder.WriteString("; media_type: ")
			builder.WriteString(attachment.MediaType)
		}
		builder.WriteString("; size_bytes: ")
		builder.WriteString(strconv.FormatInt(attachmentSizeBytes(attachment), 10))
		builder.WriteString("\n")
	}
	if len(task.Attachments) == 1 {
		builder.WriteString("If the user refers to this file without naming it, use the uploaded path above.\n")
	}

	return builder.String()
}

func attachmentSizeBytes(attachment run.TaskAttachment) int64 {
	if attachment.SizeBytes > 0 {
		return attachment.SizeBytes
	}

	return int64(len(attachment.Content))
}

func (s *runSession) finalResult(summary string) RunResult {
	return RunResult{
		Summary:       summary,
		ToolCallCount: s.toolCallCount,
		ToolCalls:     copyToolCallRecords(s.toolCalls),
		AgentTurns:    copyTurnRecords(s.turnRecords),
	}
}

func (s *runSession) recordSandboxExecResult(tool string, arguments map[string]string, result outbound.ToolResult) {
	if tool != toolNameSandboxExec {
		return
	}

	s.sandboxExecErrored = result.IsError
	if result.IsError {
		s.lastErroredSandboxExecCommand = strings.TrimSpace(arguments[toolArgumentCommand])
		s.lastSuccessfulSandboxExecCommand = ""

		return
	}
	if !result.IsError {
		s.lastErroredSandboxExecCommand = ""
		s.lastSuccessfulSandboxExecCommand = strings.TrimSpace(arguments[toolArgumentCommand])
		s.sandboxExecRetryRequired = false
		s.sandboxExecRetryMessage = ""
		s.sandboxExecRetryCorrection = ""
	}
}

func (s *runSession) partialResult() RunResult {
	return RunResult{
		ToolCallCount: s.toolCallCount,
		ToolCalls:     copyToolCallRecords(s.toolCalls),
		AgentTurns:    copyTurnRecords(s.turnRecords),
	}
}

func validateFinalSummary(summary string) error {
	if strings.TrimSpace(summary) == "" {
		return errors.New("agent final summary is required")
	}

	return nil
}

func handleUnavailableToolCall(
	session *runSession,
	turn modelTurnContext,
	parsed action,
) (RunResult, bool, error) {
	message := "tool is not available: " + parsed.Tool
	session.recordTurn(TurnRecord{
		Index:           turn.index,
		Status:          run.AgentTurnStatusInvalidToolCall,
		ActionType:      run.AgentTurnActionTypeToolCall,
		ToolName:        parsed.Tool,
		Message:         message,
		RequestMessages: turn.requestMessages,
		RawResponse:     turn.content,
		ResponseFormat:  turn.responseFormat,
		ResponsePreview: previewModelResponse(turn.content),
		StartedAt:       turn.startedAt,
		EndedAt:         turn.endedAt,
	})
	session.turns = append(session.turns,
		assistantAttemptTurn(turn.content),
		runtimeTurn(
			outbound.ModelPartKindToolCorrection,
			buildUnavailableToolCorrectionMessage(unavailableToolCorrectionRequest{
				RequestedTool:  parsed.Tool,
				AvailableTools: session.availableToolNames,
			}),
		),
	)

	return RunResult{}, false, nil
}

func handlePlaceholderToolCall(
	session *runSession,
	turn modelTurnContext,
	parsed action,
	placeholders []string,
) (RunResult, bool, error) {
	message := "tool call arguments contain placeholder values"
	session.recordTurn(TurnRecord{
		Index:           turn.index,
		Status:          run.AgentTurnStatusInvalidToolCall,
		ActionType:      run.AgentTurnActionTypeToolCall,
		ToolName:        parsed.Tool,
		Message:         message,
		RequestMessages: turn.requestMessages,
		RawResponse:     turn.content,
		ResponseFormat:  turn.responseFormat,
		ResponsePreview: previewModelResponse(turn.content),
		StartedAt:       turn.startedAt,
		EndedAt:         turn.endedAt,
	})
	session.turns = append(session.turns,
		assistantAttemptTurn(turn.content),
		runtimeTurn(
			outbound.ModelPartKindToolCorrection,
			buildPlaceholderToolArgumentCorrectionMessage(placeholderToolArgumentCorrectionRequest{
				Placeholders:    placeholders,
				AvailableTools:  session.availableToolNames,
				UploadedFileIDs: uploadedFilePaths(session.request.Task),
			}),
		),
	)

	return RunResult{}, false, nil
}

func handleStaticOutputSandboxExecToolCall(
	session *runSession,
	turn modelTurnContext,
	toolName string,
	content string,
) (RunResult, bool, error) {
	message := "sandbox_exec command only printed static text"
	session.recordTurn(TurnRecord{
		Index:           turn.index,
		Status:          run.AgentTurnStatusInvalidToolCall,
		ActionType:      run.AgentTurnActionTypeToolCall,
		ToolName:        toolName,
		Message:         message,
		RequestMessages: turn.requestMessages,
		RawResponse:     turn.content,
		ResponseFormat:  turn.responseFormat,
		ResponsePreview: previewModelResponse(turn.content),
		StartedAt:       turn.startedAt,
		EndedAt:         turn.endedAt,
	})
	session.turns = append(session.turns,
		assistantAttemptTurn(content),
		runtimeTurn(outbound.ModelPartKindToolCorrection, buildSandboxExecStaticOutputCorrectionMessage()),
	)
	session.sandboxExecRetryRequired = true
	session.sandboxExecRetryMessage = "final answer attempted after invalid sandbox_exec"
	session.sandboxExecRetryCorrection = buildSandboxExecStaticOutputCorrectionMessage()

	return RunResult{}, false, nil
}

func handleUnverifiedNumericalSandboxExecToolCall(
	session *runSession,
	turn modelTurnContext,
	toolName string,
	content string,
) (RunResult, bool, error) {
	message := "sandbox_exec numerical solve did not verify residual"
	correctionMessage := buildSandboxExecUnverifiedNumericalSolveCorrectionMessage()
	session.recordTurn(TurnRecord{
		Index:           turn.index,
		Status:          run.AgentTurnStatusInvalidToolCall,
		ActionType:      run.AgentTurnActionTypeToolCall,
		ToolName:        toolName,
		Message:         message,
		RequestMessages: turn.requestMessages,
		RawResponse:     turn.content,
		ResponseFormat:  turn.responseFormat,
		ResponsePreview: previewModelResponse(turn.content),
		StartedAt:       turn.startedAt,
		EndedAt:         turn.endedAt,
	})
	session.turns = append(session.turns,
		assistantAttemptTurn(content),
		runtimeTurn(outbound.ModelPartKindToolCorrection, correctionMessage),
	)
	session.sandboxExecRetryRequired = true
	session.sandboxExecRetryMessage = "final answer attempted after unverified sandbox_exec numerical solve"
	session.sandboxExecRetryCorrection = correctionMessage

	return RunResult{}, false, nil
}

func handleReadOnlyPDFTextOutputSandboxExecToolCall(
	session *runSession,
	turn modelTurnContext,
	toolName string,
	content string,
) (RunResult, bool, error) {
	message := "sandbox_exec pdftotext command would write beside read-only input"
	correctionMessage := buildSandboxExecReadOnlyPDFTextOutputCorrectionMessage()
	session.recordTurn(TurnRecord{
		Index:           turn.index,
		Status:          run.AgentTurnStatusInvalidToolCall,
		ActionType:      run.AgentTurnActionTypeToolCall,
		ToolName:        toolName,
		Message:         message,
		RequestMessages: turn.requestMessages,
		RawResponse:     turn.content,
		ResponseFormat:  turn.responseFormat,
		ResponsePreview: previewModelResponse(turn.content),
		StartedAt:       turn.startedAt,
		EndedAt:         turn.endedAt,
	})
	session.turns = append(session.turns,
		assistantAttemptTurn(content),
		runtimeTurn(outbound.ModelPartKindToolCorrection, correctionMessage),
	)
	session.sandboxExecRetryRequired = true
	session.sandboxExecRetryMessage = "final answer attempted after invalid sandbox_exec pdftotext command"
	session.sandboxExecRetryCorrection = correctionMessage

	return RunResult{}, false, nil
}

func handleRepeatedFailedSandboxExecToolCall(
	session *runSession,
	turn modelTurnContext,
	toolName string,
	content string,
) (RunResult, bool, error) {
	message := "sandbox_exec repeated a failed command unchanged"
	correctionMessage := buildSandboxExecRepeatedFailedCommandCorrectionMessage()
	session.recordTurn(TurnRecord{
		Index:           turn.index,
		Status:          run.AgentTurnStatusInvalidToolCall,
		ActionType:      run.AgentTurnActionTypeToolCall,
		ToolName:        toolName,
		Message:         message,
		RequestMessages: turn.requestMessages,
		RawResponse:     turn.content,
		ResponseFormat:  turn.responseFormat,
		ResponsePreview: previewModelResponse(turn.content),
		StartedAt:       turn.startedAt,
		EndedAt:         turn.endedAt,
	})
	session.turns = append(session.turns,
		assistantAttemptTurn(content),
		runtimeTurn(outbound.ModelPartKindToolCorrection, correctionMessage),
	)
	session.sandboxExecRetryRequired = true
	session.sandboxExecRetryMessage = "final answer attempted after repeated failed sandbox_exec command"
	session.sandboxExecRetryCorrection = correctionMessage

	return RunResult{}, false, nil
}

func handleRepeatedSuccessfulSandboxExecToolCall(
	session *runSession,
	turn modelTurnContext,
	toolName string,
	content string,
) (RunResult, bool, error) {
	message := "sandbox_exec repeated a successful command unchanged"
	correctionMessage := buildSandboxExecRepeatedSuccessfulCommandCorrectionMessage()
	session.recordTurn(TurnRecord{
		Index:           turn.index,
		Status:          run.AgentTurnStatusInvalidToolCall,
		ActionType:      run.AgentTurnActionTypeToolCall,
		ToolName:        toolName,
		Message:         message,
		RequestMessages: turn.requestMessages,
		RawResponse:     turn.content,
		ResponseFormat:  turn.responseFormat,
		ResponsePreview: previewModelResponse(turn.content),
		StartedAt:       turn.startedAt,
		EndedAt:         turn.endedAt,
	})
	session.turns = append(session.turns,
		assistantAttemptTurn(content),
		runtimeTurn(outbound.ModelPartKindToolCorrection, correctionMessage),
	)

	return RunResult{}, false, nil
}

func uploadedFilePaths(task run.TaskSpec) []string {
	if len(task.Attachments) == 0 {
		return nil
	}

	paths := make([]string, 0, len(task.Attachments))
	for _, attachment := range task.Attachments {
		paths = append(paths, attachment.Filename)
	}

	return paths
}

func sandboxExecCommandOnlyPrintsStaticText(tool string, arguments map[string]string) bool {
	if strings.TrimSpace(tool) != toolNameSandboxExec {
		return false
	}
	command := strings.TrimSpace(arguments[toolArgumentCommand])
	if command == "" || sandboxExecCommandHasDynamicShellExpansion(command) {
		return false
	}

	lower := strings.ToLower(command)
	switch {
	case strings.HasPrefix(lower, "echo "):
		return true
	case strings.HasPrefix(lower, "printf "):
		return true
	case strings.HasPrefix(lower, "awk ") && sandboxExecAwkPrintsQuotedLiteral(lower):
		return true
	case strings.Contains(lower, "python") && sandboxExecPythonPrintsQuotedLiteral(lower):
		return true
	default:
		return false
	}
}

func sandboxExecCommandUsesUnverifiedNumericalSolve(tool string, arguments map[string]string) bool {
	if strings.TrimSpace(tool) != toolNameSandboxExec {
		return false
	}
	command := strings.ToLower(strings.TrimSpace(arguments[toolArgumentCommand]))
	if command == "" {
		return false
	}
	if strings.Contains(command, "brentq") || strings.Contains(command, "bisect") {
		return false
	}

	return sandboxExecCommandUsesNumericalSolver(command) && !sandboxExecCommandPrintsResidual(command)
}

func sandboxExecCommandWritesReadOnlyPDFTextOutput(tool string, arguments map[string]string) bool {
	if strings.TrimSpace(tool) != toolNameSandboxExec {
		return false
	}
	command := strings.TrimSpace(arguments[toolArgumentCommand])
	if command == "" || !strings.Contains(strings.ToLower(command), "pdftotext") {
		return false
	}

	tokens := shellLikeTokens(command)
	for index, token := range tokens {
		if !sandboxExecIsPDFToTextToken(token) {
			continue
		}
		if sandboxExecPDFToTextInvocationWritesReadOnlyInput(tokens[index+1:]) {
			return true
		}
	}

	return false
}

func sandboxExecCommandRepeatsFailedCommand(
	session *runSession,
	tool string,
	arguments map[string]string,
) bool {
	if !session.sandboxExecErrored || strings.TrimSpace(tool) != toolNameSandboxExec {
		return false
	}
	command := strings.TrimSpace(arguments[toolArgumentCommand])
	if command == "" {
		return false
	}

	return command == session.lastErroredSandboxExecCommand
}

func sandboxExecCommandRepeatsSuccessfulCommand(
	session *runSession,
	tool string,
	arguments map[string]string,
) bool {
	if session.sandboxExecErrored || strings.TrimSpace(tool) != toolNameSandboxExec {
		return false
	}
	command := strings.TrimSpace(arguments[toolArgumentCommand])
	if command == "" || session.lastSuccessfulSandboxExecCommand == "" {
		return false
	}

	return command == session.lastSuccessfulSandboxExecCommand
}

func sandboxExecPDFToTextInvocationWritesReadOnlyInput(arguments []string) bool {
	for index, argument := range arguments {
		if shellCommandBoundaryToken(argument) {
			return false
		}
		if workspaceInputPDFPath(argument) && pdftotextOutputWritesReadOnlyInput(arguments, index+1) {
			return true
		}
	}

	return false
}

func pdftotextOutputWritesReadOnlyInput(arguments []string, outputIndex int) bool {
	if outputIndex >= len(arguments) {
		return true
	}
	output := arguments[outputIndex]
	if shellCommandBoundaryToken(output) || shellRedirectionToken(output) {
		return true
	}
	if allDigits(output) && nextTokenIsShellRedirection(arguments, outputIndex) {
		return true
	}
	if output == "-" {
		return false
	}

	return strings.HasPrefix(output, "/workspace/input/") || strings.HasPrefix(output, "-")
}

func nextTokenIsShellRedirection(arguments []string, index int) bool {
	nextIndex := index + 1
	if nextIndex >= len(arguments) {
		return false
	}

	return shellRedirectionToken(arguments[nextIndex])
}

func sandboxExecIsPDFToTextToken(token string) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	if slash := strings.LastIndex(token, "/"); slash >= 0 {
		token = token[slash+1:]
	}

	return token == "pdftotext"
}

func workspaceInputPDFPath(token string) bool {
	return strings.Contains(token, "/workspace/input/") && strings.HasSuffix(strings.ToLower(token), ".pdf")
}

func shellCommandBoundaryToken(token string) bool {
	switch token {
	case "|", "&", ";":
		return true
	default:
		return false
	}
}

func shellRedirectionToken(token string) bool {
	switch token {
	case ">", ">>", "<", "<<":
		return true
	default:
		return false
	}
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}

	return true
}

func shellLikeTokens(command string) []string {
	tokenizer := shellLikeTokenizer{}
	for _, char := range command {
		tokenizer.write(char)
	}

	return tokenizer.tokens()
}

type shellLikeTokenizer struct {
	items   []string
	current strings.Builder
	quote   rune
	escaped bool
}

func (t *shellLikeTokenizer) write(char rune) {
	if t.writeEscaped(char) {
		return
	}
	if t.startEscape(char) {
		return
	}
	if t.writeQuoted(char) {
		return
	}
	t.writeUnquoted(char)
}

func (t *shellLikeTokenizer) writeEscaped(char rune) bool {
	if !t.escaped {
		return false
	}
	t.current.WriteRune(char)
	t.escaped = false

	return true
}

func (t *shellLikeTokenizer) startEscape(char rune) bool {
	if char != '\\' || t.quote == '\'' {
		return false
	}
	t.escaped = true

	return true
}

func (t *shellLikeTokenizer) writeQuoted(char rune) bool {
	if t.quote == 0 {
		return false
	}
	if char == t.quote {
		t.quote = 0

		return true
	}
	t.current.WriteRune(char)

	return true
}

func (t *shellLikeTokenizer) writeUnquoted(char rune) {
	switch char {
	case '\'', '"':
		t.quote = char
	case ' ', '\t', '\n', '\r':
		t.flush()
	case '|', '&', ';', '>', '<':
		t.flush()
		t.items = append(t.items, string(char))
	default:
		t.current.WriteRune(char)
	}
}

func (t *shellLikeTokenizer) tokens() []string {
	if t.escaped {
		t.current.WriteRune('\\')
	}
	t.flush()

	return t.items
}

func (t *shellLikeTokenizer) flush() {
	if t.current.Len() == 0 {
		return
	}
	t.items = append(t.items, t.current.String())
	t.current.Reset()
}

func sandboxExecCommandUsesNumericalSolver(command string) bool {
	return strings.Contains(command, "fsolve") ||
		strings.Contains(command, "scipy.optimize.root") ||
		strings.Contains(command, "optimize.root(")
}

func sandboxExecCommandPrintsResidual(command string) bool {
	return strings.Contains(command, "residual") ||
		strings.Contains(command, "f(root") ||
		strings.Contains(command, "f(x)") ||
		strings.Contains(command, "f(mid)") ||
		strings.Contains(command, "f(m)")
}

func sandboxExecCommandHasDynamicShellExpansion(command string) bool {
	return strings.Contains(command, "$(") ||
		strings.Contains(command, "${") ||
		strings.Contains(command, "$((") ||
		strings.Contains(command, "`")
}

func sandboxExecAwkPrintsQuotedLiteral(command string) bool {
	if !strings.Contains(command, "begin") {
		return false
	}

	return printCallHasOnlyQuotedLiteral(command, `print "`, `"`, "}") ||
		printCallHasOnlyQuotedLiteral(command, `print \"`, `\"`, "}") ||
		printCallHasOnlyQuotedLiteral(command, "print '", "'", "}")
}

func sandboxExecPythonPrintsQuotedLiteral(command string) bool {
	return printCallHasOnlyQuotedLiteral(command, `print("`, `"`, ")") ||
		printCallHasOnlyQuotedLiteral(command, `print('`, "'", ")") ||
		printCallHasOnlyQuotedLiteral(command, `print(\"`, `\"`, ")") ||
		printCallHasOnlyQuotedLiteral(command, `print(\'`, `\'`, ")")
}

func printCallHasOnlyQuotedLiteral(command string, open string, closeToken string, terminator string) bool {
	start := strings.Index(command, open)
	if start < 0 {
		return false
	}
	afterOpen := command[start+len(open):]
	closeIndex := strings.Index(afterOpen, closeToken)
	if closeIndex < 0 {
		return false
	}
	afterClose := strings.TrimLeft(afterOpen[closeIndex+len(closeToken):], " \t;")
	if strings.HasPrefix(afterClose, terminator) {
		return true
	}
	if terminator == "}" && strings.HasPrefix(afterClose, `}`) {
		return true
	}

	return false
}

func (r *Runner) handleToolCall(ctx context.Context, session *runSession, content string, parsed action) (RunResult, bool, error) {
	if strings.TrimSpace(parsed.Tool) == "" {
		session.turns = append(session.turns,
			assistantAttemptTurn(content),
			runtimeTurn(outbound.ModelPartKindToolCorrection, "Tool error:\nmissing tool name"),
		)
		return RunResult{}, false, nil
	}
	session.turns = append(session.turns, assistantTurn(content))

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
	session.recordSandboxExecResult(parsed.Tool, parsed.Arguments, result)

	session.turns = append(session.turns, toolResultTurn([]outbound.ModelPart{{
		Kind:       outbound.ModelPartKindToolResult,
		Text:       buildToolObservation(parsed.Tool, parsed.Arguments, result),
		ToolName:   parsed.Tool,
		IsError:    result.IsError,
		ToolCallID: legacyToolCallID(parsed.Tool),
	}}))

	return RunResult{}, false, nil
}

func (r *Runner) handleNativeToolCalls(
	ctx context.Context,
	session *runSession,
	turn modelTurnContext,
	calls []outbound.ModelToolCall,
) (RunResult, bool, error) {
	normalized := normalizeModelToolCalls(calls)
	if len(normalized) == 0 {
		return session.partialResult(), false, newAgentError(
			ErrorCodeAgentProtocolError,
			"",
			errors.New("native tool call response had no usable tool calls"),
		)
	}
	for _, call := range normalized {
		if !session.toolIsAvailable(call.Name) {
			return handleUnavailableNativeToolCall(session, turn, call)
		}
		if placeholders := placeholderArgumentValues(call.Arguments); len(placeholders) > 0 {
			return handlePlaceholderNativeToolCall(session, turn, call, placeholders)
		}
		if sandboxExecCommandOnlyPrintsStaticText(call.Name, call.Arguments) {
			return handleStaticOutputSandboxExecToolCall(session, turn, call.Name, turn.content)
		}
		if sandboxExecCommandUsesUnverifiedNumericalSolve(call.Name, call.Arguments) {
			return handleUnverifiedNumericalSandboxExecToolCall(session, turn, call.Name, turn.content)
		}
		if sandboxExecCommandWritesReadOnlyPDFTextOutput(call.Name, call.Arguments) {
			return handleReadOnlyPDFTextOutputSandboxExecToolCall(session, turn, call.Name, turn.content)
		}
		if sandboxExecCommandRepeatsFailedCommand(session, call.Name, call.Arguments) {
			return handleRepeatedFailedSandboxExecToolCall(session, turn, call.Name, turn.content)
		}
		if sandboxExecCommandRepeatsSuccessfulCommand(session, call.Name, call.Arguments) {
			return handleRepeatedSuccessfulSandboxExecToolCall(session, turn, call.Name, turn.content)
		}
	}

	session.recordTurn(TurnRecord{
		Index:           turn.index,
		Status:          run.AgentTurnStatusToolCall,
		ActionType:      run.AgentTurnActionTypeToolCall,
		ToolName:        joinedToolCallNames(normalized),
		Message:         "model requested native tool call",
		RequestMessages: turn.requestMessages,
		RawResponse:     turn.content,
		ResponseFormat:  turn.responseFormat,
		ResponsePreview: previewModelResponse(turn.content),
		StartedAt:       turn.startedAt,
		EndedAt:         turn.endedAt,
	})
	session.turns = append(session.turns, assistantToolCallTurn("", normalized))

	resultParts := make([]outbound.ModelPart, 0, len(normalized))
	for _, call := range normalized {
		startedAt := r.clock()
		result, err := r.tools.RunTool(ctx, outbound.ToolCall{
			RunID:     session.request.RunID,
			Context:   session.request.Context,
			Name:      call.Name,
			Arguments: call.Arguments,
		})
		endedAt := r.clock()
		if err != nil {
			session.recordToolCall(call.Name, call.Arguments, "tool execution failed", true, startedAt, endedAt)

			return session.partialResult(), false, newAgentError(ErrorCodeToolExecutionFailed, "", err)
		}
		session.toolCallCount++
		session.recordToolCall(call.Name, call.Arguments, result.Content, result.IsError, startedAt, endedAt)
		session.recordSandboxExecResult(call.Name, call.Arguments, result)
		resultParts = append(resultParts, outbound.ModelPart{
			Kind:       outbound.ModelPartKindToolResult,
			Text:       buildToolObservation(call.Name, call.Arguments, result),
			ToolCallID: call.ID,
			ToolName:   call.Name,
			IsError:    result.IsError,
		})
	}
	session.turns = append(session.turns, toolResultTurn(resultParts))

	return RunResult{}, false, nil
}

func handleUnavailableNativeToolCall(
	session *runSession,
	turn modelTurnContext,
	call outbound.ModelToolCall,
) (RunResult, bool, error) {
	message := "tool is not available: " + call.Name
	session.recordTurn(TurnRecord{
		Index:           turn.index,
		Status:          run.AgentTurnStatusInvalidToolCall,
		ActionType:      run.AgentTurnActionTypeToolCall,
		ToolName:        call.Name,
		Message:         message,
		RequestMessages: turn.requestMessages,
		RawResponse:     turn.content,
		ResponseFormat:  turn.responseFormat,
		ResponsePreview: previewModelResponse(turn.content),
		StartedAt:       turn.startedAt,
		EndedAt:         turn.endedAt,
	})
	session.turns = append(session.turns,
		assistantAttemptTurn(turn.content),
		runtimeTurn(
			outbound.ModelPartKindToolCorrection,
			buildUnavailableToolCorrectionMessage(unavailableToolCorrectionRequest{
				RequestedTool:  call.Name,
				AvailableTools: session.availableToolNames,
			}),
		),
	)

	return RunResult{}, false, nil
}

func handlePlaceholderNativeToolCall(
	session *runSession,
	turn modelTurnContext,
	call outbound.ModelToolCall,
	placeholders []string,
) (RunResult, bool, error) {
	message := "tool call arguments contain placeholder values"
	session.recordTurn(TurnRecord{
		Index:           turn.index,
		Status:          run.AgentTurnStatusInvalidToolCall,
		ActionType:      run.AgentTurnActionTypeToolCall,
		ToolName:        call.Name,
		Message:         message,
		RequestMessages: turn.requestMessages,
		RawResponse:     turn.content,
		ResponseFormat:  turn.responseFormat,
		ResponsePreview: previewModelResponse(turn.content),
		StartedAt:       turn.startedAt,
		EndedAt:         turn.endedAt,
	})
	session.turns = append(session.turns,
		assistantAttemptTurn(turn.content),
		runtimeTurn(
			outbound.ModelPartKindToolCorrection,
			buildPlaceholderToolArgumentCorrectionMessage(placeholderToolArgumentCorrectionRequest{
				Placeholders:    placeholders,
				AvailableTools:  session.availableToolNames,
				UploadedFileIDs: uploadedFilePaths(session.request.Task),
			}),
		),
	)

	return RunResult{}, false, nil
}

func (s *runSession) toolIsAvailable(name string) bool {
	if s.availableTools == nil {
		return false
	}

	_, ok := s.availableTools[name]

	return ok
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
	record.RequestMessages = copyTurnMessageRecords(record.RequestMessages)
	record.RawResponse = previewRawResponse(record.RawResponse)
	record.ResponsePreview = previewModelResponse(record.ResponsePreview)
	record.CorrectionMessage = previewCorrectionMessage(record.CorrectionMessage)
	s.turnRecords = append(s.turnRecords, record)
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
		item.RequestMessages = copyTurnMessageRecords(record.RequestMessages)
		item.RawResponse = previewRawResponse(record.RawResponse)
		item.ResponsePreview = previewModelResponse(record.ResponsePreview)
		item.CorrectionMessage = previewCorrectionMessage(record.CorrectionMessage)
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

func copyToolDefinitions(definitions []outbound.ToolDefinition) []outbound.ToolDefinition {
	if len(definitions) == 0 {
		return nil
	}

	copied := make([]outbound.ToolDefinition, 0, len(definitions))
	for _, definition := range definitions {
		item := definition
		if len(definition.Arguments) > 0 {
			item.Arguments = append([]outbound.ToolArgumentDefinition(nil), definition.Arguments...)
		}
		copied = append(copied, item)
	}

	return copied
}

func normalizeModelToolCalls(calls []outbound.ModelToolCall) []outbound.ModelToolCall {
	if len(calls) == 0 {
		return nil
	}

	normalized := make([]outbound.ModelToolCall, 0, len(calls))
	for index, call := range calls {
		name := strings.TrimSpace(call.Name)
		if name == "" {
			continue
		}
		id := strings.TrimSpace(call.ID)
		if id == "" {
			id = generatedToolCallID(name, index)
		}
		normalized = append(normalized, outbound.ModelToolCall{
			ID:        id,
			Name:      name,
			Arguments: copyArguments(call.Arguments),
		})
	}

	return normalized
}

func generatedToolCallID(name string, index int) string {
	normalized := strings.Map(func(char rune) rune {
		switch {
		case char >= 'a' && char <= 'z':
			return char
		case char >= 'A' && char <= 'Z':
			return char + ('a' - 'A')
		case char >= '0' && char <= '9':
			return char
		default:
			return '_'
		}
	}, name)
	normalized = strings.Trim(normalized, "_")
	if normalized == "" {
		normalized = "tool"
	}

	return normalized + "_" + strconv.Itoa(index+1)
}

func legacyToolCallID(name string) string {
	return generatedToolCallID(name, 0)
}

func joinedToolCallNames(calls []outbound.ModelToolCall) string {
	if len(calls) == 0 {
		return ""
	}

	names := make([]string, 0, len(calls))
	for _, call := range calls {
		if name := strings.TrimSpace(call.Name); name != "" {
			names = append(names, name)
		}
	}

	return strings.Join(names, ",")
}

func nativeToolCallsRawResponse(calls []outbound.ModelToolCall) string {
	type nativeToolCallRecord struct {
		ID        string            `json:"id,omitempty"`
		Tool      string            `json:"tool"`
		Arguments map[string]string `json:"arguments,omitempty"`
	}
	payload := struct {
		Type      string                 `json:"type"`
		ToolCalls []nativeToolCallRecord `json:"tool_calls"`
	}{
		Type:      string(outbound.ModelPartKindToolCall),
		ToolCalls: make([]nativeToolCallRecord, 0, len(calls)),
	}
	for _, call := range calls {
		name := strings.TrimSpace(call.Name)
		if name == "" {
			continue
		}
		payload.ToolCalls = append(payload.ToolCalls, nativeToolCallRecord{
			ID:        strings.TrimSpace(call.ID),
			Tool:      name,
			Arguments: copyArguments(call.Arguments),
		})
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return `{"type":"tool_call","tool_calls":[]}`
	}

	return string(encoded)
}

func responseRequestMessages(response outbound.ModelResponse, fallback []TurnMessageRecord) []TurnMessageRecord {
	messages := copyProviderRequestMessages(response.RequestMessages)
	if len(messages) == 0 {
		return fallback
	}

	return messages
}

func copyProviderRequestMessages(messages []outbound.ModelRequestMessage) []TurnMessageRecord {
	if len(messages) == 0 {
		return nil
	}

	copied := make([]TurnMessageRecord, 0, len(messages))
	for _, message := range messages {
		copied = append(copied, TurnMessageRecord{
			Role:       message.Role,
			Kind:       message.Kind,
			Content:    message.Content,
			ToolCallID: message.ToolCallID,
			ToolName:   message.ToolName,
		})
	}

	return copyTurnMessageRecords(copied)
}

func copyModelRequestMessages(instructions string, turns []outbound.ModelTurn) []TurnMessageRecord {
	var copied []TurnMessageRecord
	if strings.TrimSpace(instructions) != "" {
		copied = append(copied, TurnMessageRecord{
			Role:    string(outbound.ModelRoleRuntime),
			Kind:    "instructions",
			Content: instructions,
		})
	}
	for _, turn := range turns {
		role := string(turn.Role)
		for _, part := range turn.Parts {
			copied = append(copied, TurnMessageRecord{
				Role:       role,
				Kind:       string(part.Kind),
				Content:    part.Text,
				ToolCallID: part.ToolCallID,
				ToolName:   part.ToolName,
			})
		}
	}

	return copyTurnMessageRecords(copied)
}

func copyTurnMessageRecords(messages []TurnMessageRecord) []TurnMessageRecord {
	if len(messages) == 0 {
		return nil
	}

	copied := make([]TurnMessageRecord, 0, len(messages))
	for _, message := range messages {
		role := strings.TrimSpace(message.Role)
		if role == "" {
			continue
		}

		copied = append(copied, TurnMessageRecord{
			Role:       role,
			Kind:       strings.TrimSpace(message.Kind),
			Content:    previewRequestMessageContent(message.Content),
			ToolCallID: strings.TrimSpace(message.ToolCallID),
			ToolName:   strings.TrimSpace(message.ToolName),
		})
	}

	return copied
}

func availableToolMap(tools []outbound.ToolDefinition) map[string]outbound.ToolDefinition {
	if len(tools) == 0 {
		return map[string]outbound.ToolDefinition{}
	}

	indexed := make(map[string]outbound.ToolDefinition, len(tools))
	for _, tool := range tools {
		indexed[tool.Name] = tool
	}

	return indexed
}

func availableToolNames(tools []outbound.ToolDefinition) []string {
	if len(tools) == 0 {
		return nil
	}

	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)

	return names
}

func placeholderArgumentValues(arguments map[string]string) []string {
	if len(arguments) == 0 {
		return nil
	}

	var placeholders []string
	for name, value := range arguments {
		for _, token := range placeholderTokens(value) {
			placeholders = append(placeholders, name+"="+token)
		}
	}
	sort.Strings(placeholders)

	return placeholders
}

func placeholderTokens(value string) []string {
	var tokens []string
	remaining := value
	for {
		start := strings.Index(remaining, "<")
		if start < 0 {
			break
		}
		remaining = remaining[start+1:]
		end := strings.Index(remaining, ">")
		if end < 0 {
			break
		}
		name := remaining[:end]
		token := "<" + name + ">"
		if isPlaceholderName(name) {
			tokens = append(tokens, token)
		}
		remaining = remaining[end+1:]
	}

	return tokens
}

func isPlaceholderName(name string) bool {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if normalized == "" || len(normalized) > 64 {
		return false
	}
	for _, char := range normalized {
		if (char >= 'a' && char <= 'z') ||
			(char >= '0' && char <= '9') ||
			char == '_' ||
			char == '-' ||
			char == ' ' {
			continue
		}

		return false
	}

	compact := strings.NewReplacer("_", "", "-", "", " ", "").Replace(normalized)
	switch compact {
	case toolArgumentCommand, "filename", "filepath", "key", "path", "toolname", "value":
		return true
	default:
		return strings.Contains(compact, "file") || strings.Contains(compact, "path")
	}
}

func classifyModelResponseFormat(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return modelResponseFormatEmpty
	}
	if strings.HasPrefix(trimmed, "```") {
		return modelResponseFormatMarkdownFence
	}

	decoded, extraErr, decodeErr := decodeSingleJSONValue(trimmed)
	if decodeErr != nil {
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			return modelResponseFormatInvalidJSON
		}

		return modelResponseFormatPlainText
	}
	if !errors.Is(extraErr, io.EOF) {
		return modelResponseFormatMultipleJSONValues
	}

	switch decoded.(type) {
	case map[string]any:
		return modelResponseFormatJSONObject
	case []any:
		return modelResponseFormatJSONArray
	default:
		return modelResponseFormatJSONScalar
	}
}

func previewModelResponse(content string) string {
	return truncateAgentTurnText(content, run.MaxAgentTurnPreviewLength)
}

func previewRawResponse(content string) string {
	return truncateAgentTurnText(content, run.MaxAgentTurnRawResponseLength)
}

func previewRequestMessageContent(content string) string {
	return truncateAgentTurnText(content, run.MaxAgentTurnMessageContentLength)
}

func boundedAssistantAttempt(content string) string {
	return truncateAgentTurnText(content, run.MaxAgentTurnPreviewLength)
}

func previewCorrectionMessage(content string) string {
	return truncateAgentTurnText(strings.TrimSpace(content), run.MaxAgentTurnCorrectionLength)
}

func truncateAgentTurnText(content string, maxLength int) string {
	content = strings.ToValidUTF8(content, "\uFFFD")
	if len(content) <= maxLength {
		return content
	}

	maxContentLength := maxLength - len(agentTurnPreviewTruncatedMarker)
	if maxContentLength < 0 {
		maxContentLength = 0
	}

	for maxContentLength > 0 && !utf8.ValidString(content[:maxContentLength]) {
		maxContentLength--
	}

	return content[:maxContentLength] + agentTurnPreviewTruncatedMarker
}
