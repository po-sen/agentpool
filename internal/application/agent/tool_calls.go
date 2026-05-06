package agent

import (
	"context"
	"errors"
	"strings"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

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
