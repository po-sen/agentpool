package agent

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

const defaultMaxTurns = 4

var errToolRunnerRequired = errors.New("tool runner is required")

const agentTurnPreviewTruncatedMarker = "\n... [truncated]"

const (
	modelRoleSystem    = "system"
	modelRoleUser      = "user"
	modelRoleAssistant = "assistant"
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
	SystemPrompt  string
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
	request            RunRequest
	systemPrompt       string
	availableTools     map[string]outbound.ToolDefinition
	availableToolNames []string
	messages           []outbound.ModelMessage
	turnIndex          int
	toolCallCount      int
	toolCalls          []ToolCallRecord
	turns              []TurnRecord
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
		availableToolNames: availableToolNames(tools),
		messages: []outbound.ModelMessage{
			{Role: modelRoleSystem, Content: systemPrompt},
			{Role: modelRoleUser, Content: buildTaskMessage(request.Task)},
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
		message := "model response did not match AgentPool action protocol"
		if parsed.parseErr.Message != "" {
			message = parsed.parseErr.Message
		}
		session.recordTurn(TurnRecord{
			Index:           turnIndex,
			Status:          run.AgentTurnStatusProtocolError,
			Message:         message,
			ResponsePreview: previewModelResponse(response.Content),
			StartedAt:       startedAt,
			EndedAt:         endedAt,
		})
		session.messages = append(session.messages,
			outbound.ModelMessage{Role: modelRoleAssistant, Content: response.Content},
			outbound.ModelMessage{Role: modelRoleUser, Content: buildProtocolCorrectionMessage(parsed.parseErr)},
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
		if !session.toolIsAvailable(parsed.Tool) {
			return handleUnavailableToolCall(session, content, parsed, turnIndex, startedAt, endedAt)
		}
		if placeholders := placeholderArgumentValues(parsed.Arguments); len(placeholders) > 0 {
			return handlePlaceholderToolCall(session, content, parsed, placeholders, turnIndex, startedAt, endedAt)
		}

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

func buildTaskMessage(task run.TaskSpec) string {
	if len(task.Attachments) == 0 {
		return task.Prompt
	}

	var builder strings.Builder
	builder.WriteString(task.Prompt)
	builder.WriteString("\n\nUploaded files available in the workspace:\n")
	for _, attachment := range task.Attachments {
		builder.WriteString("- path: ")
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
		AgentTurns:    copyTurnRecords(s.turns),
		SystemPrompt:  s.systemPrompt,
	}
}

func (s *runSession) partialResult() RunResult {
	return RunResult{
		ToolCallCount: s.toolCallCount,
		ToolCalls:     copyToolCallRecords(s.toolCalls),
		AgentTurns:    copyTurnRecords(s.turns),
		SystemPrompt:  s.systemPrompt,
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
	content string,
	parsed action,
	turnIndex int,
	startedAt time.Time,
	endedAt time.Time,
) (RunResult, bool, error) {
	message := "tool is not available: " + parsed.Tool
	session.recordTurn(TurnRecord{
		Index:           turnIndex,
		Status:          run.AgentTurnStatusInvalidToolCall,
		ActionType:      run.AgentTurnActionTypeToolCall,
		ToolName:        parsed.Tool,
		Message:         message,
		ResponsePreview: previewModelResponse(content),
		StartedAt:       startedAt,
		EndedAt:         endedAt,
	})
	session.messages = append(session.messages,
		outbound.ModelMessage{Role: modelRoleAssistant, Content: content},
		outbound.ModelMessage{
			Role: modelRoleUser,
			Content: buildUnavailableToolCorrectionMessage(unavailableToolCorrectionRequest{
				RequestedTool:  parsed.Tool,
				AvailableTools: session.availableToolNames,
			}),
		},
	)

	return RunResult{}, false, nil
}

func handlePlaceholderToolCall(
	session *runSession,
	content string,
	parsed action,
	placeholders []string,
	turnIndex int,
	startedAt time.Time,
	endedAt time.Time,
) (RunResult, bool, error) {
	message := "tool call arguments contain placeholder values"
	session.recordTurn(TurnRecord{
		Index:           turnIndex,
		Status:          run.AgentTurnStatusInvalidToolCall,
		ActionType:      run.AgentTurnActionTypeToolCall,
		ToolName:        parsed.Tool,
		Message:         message,
		ResponsePreview: previewModelResponse(content),
		StartedAt:       startedAt,
		EndedAt:         endedAt,
	})
	session.messages = append(session.messages,
		outbound.ModelMessage{Role: modelRoleAssistant, Content: content},
		outbound.ModelMessage{
			Role: modelRoleUser,
			Content: buildPlaceholderToolArgumentCorrectionMessage(placeholderToolArgumentCorrectionRequest{
				Placeholders:    placeholders,
				AvailableTools:  session.availableToolNames,
				UploadedFileIDs: uploadedFilePaths(session.request.Task),
			}),
		},
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

func (r *Runner) handleToolCall(ctx context.Context, session *runSession, content string, parsed action) (RunResult, bool, error) {
	session.messages = append(session.messages, outbound.ModelMessage{Role: modelRoleAssistant, Content: content})
	if strings.TrimSpace(parsed.Tool) == "" {
		session.messages = append(session.messages, outbound.ModelMessage{
			Role:    modelRoleUser,
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
		Role:    modelRoleUser,
		Content: buildToolObservation(parsed.Tool, result),
	})

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
	case "command", "filename", "filepath", "key", "path", "toolname", "value":
		return true
	default:
		return strings.Contains(compact, "file") || strings.Contains(compact, "path")
	}
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
