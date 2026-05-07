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
	correctionMessage := buildUnavailableToolCorrectionMessage(unavailableToolCorrectionRequest{
		RequestedTool:  parsed.Tool,
		AvailableTools: session.availableToolNames,
	})
	session.recordTurn(TurnRecord{
		Index:             turn.index,
		Status:            run.AgentTurnStatusInvalidToolCall,
		ActionType:        run.AgentTurnActionTypeToolCall,
		ToolName:          parsed.Tool,
		Message:           message,
		RequestMessages:   turn.requestMessages,
		RawResponse:       turn.content,
		ResponseFormat:    turn.responseFormat,
		CorrectionMessage: correctionMessage,
		ResponsePreview:   previewModelResponse(turn.content),
		StartedAt:         turn.startedAt,
		EndedAt:           turn.endedAt,
	})
	session.turns = append(session.turns,
		assistantAttemptTurn(turn.content),
		runtimeTurn(outbound.ModelPartKindToolCorrection, correctionMessage),
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
	correctionMessage := buildPlaceholderToolArgumentCorrectionMessage(placeholderToolArgumentCorrectionRequest{
		Placeholders:    placeholders,
		AvailableTools:  session.availableToolNames,
		UploadedFileIDs: uploadedFilePaths(session.request.Task),
	})
	session.recordTurn(TurnRecord{
		Index:             turn.index,
		Status:            run.AgentTurnStatusInvalidToolCall,
		ActionType:        run.AgentTurnActionTypeToolCall,
		ToolName:          parsed.Tool,
		Message:           message,
		RequestMessages:   turn.requestMessages,
		RawResponse:       turn.content,
		ResponseFormat:    turn.responseFormat,
		CorrectionMessage: correctionMessage,
		ResponsePreview:   previewModelResponse(turn.content),
		StartedAt:         turn.startedAt,
		EndedAt:           turn.endedAt,
	})
	session.turns = append(session.turns,
		assistantAttemptTurn(turn.content),
		runtimeTurn(outbound.ModelPartKindToolCorrection, correctionMessage),
	)

	return RunResult{}, false, nil
}

func handlePrematureFinalAfterToolError(
	session *runSession,
	turn modelTurnContext,
) (RunResult, bool, error) {
	correctionMessage := buildPrematureFinalAfterToolErrorCorrectionMessage(prematureFinalAfterToolErrorCorrectionRequest{
		AvailableTools: session.availableToolNames,
	})
	session.recordTurn(TurnRecord{
		Index:             turn.index,
		Status:            run.AgentTurnStatusProtocolError,
		ActionType:        run.AgentTurnActionTypeFinal,
		Message:           "model returned final immediately after tool error",
		RequestMessages:   turn.requestMessages,
		RawResponse:       turn.content,
		ResponseFormat:    turn.responseFormat,
		ProtocolErrorCode: "premature_final_after_tool_error",
		CorrectionMessage: correctionMessage,
		ResponsePreview:   previewModelResponse(turn.content),
		StartedAt:         turn.startedAt,
		EndedAt:           turn.endedAt,
	})
	session.turns = append(session.turns,
		assistantAttemptTurn(turn.content),
		runtimeTurn(outbound.ModelPartKindToolCorrection, correctionMessage),
	)

	return RunResult{}, false, nil
}

func handleRepeatedToolCall(
	session *runSession,
	turn modelTurnContext,
	toolName string,
	arguments map[string]string,
) (RunResult, bool, error) {
	correctionMessage := buildRepeatedToolCallCorrectionMessage(repeatedToolCallCorrectionRequest{
		Tool:           toolName,
		Arguments:      arguments,
		AvailableTools: session.availableToolNames,
	})
	session.recordTurn(TurnRecord{
		Index:             turn.index,
		Status:            run.AgentTurnStatusInvalidToolCall,
		ActionType:        run.AgentTurnActionTypeToolCall,
		ToolName:          toolName,
		Message:           "tool call repeats a previous invocation",
		RequestMessages:   turn.requestMessages,
		RawResponse:       turn.content,
		ResponseFormat:    turn.responseFormat,
		CorrectionMessage: correctionMessage,
		ResponsePreview:   previewModelResponse(turn.content),
		StartedAt:         turn.startedAt,
		EndedAt:           turn.endedAt,
	})
	session.turns = append(session.turns,
		assistantAttemptTurn(turn.content),
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

func (r *Runner) handleToolCall(ctx context.Context, session *runSession, content string, parsed action) (RunResult, bool, error) {
	if strings.TrimSpace(parsed.Tool) == "" {
		session.turns = append(session.turns,
			assistantAttemptTurn(content),
			runtimeTurn(outbound.ModelPartKindToolCorrection, "Tool error:\nmissing tool name"),
		)
		return RunResult{}, false, nil
	}
	modelCalls := normalizeModelToolCalls([]outbound.ModelToolCall{{
		Name:      parsed.Tool,
		Arguments: parsed.Arguments,
	}})
	if len(modelCalls) == 0 {
		session.turns = append(session.turns,
			assistantAttemptTurn(content),
			runtimeTurn(outbound.ModelPartKindToolCorrection, "Tool error:\nmissing usable tool call"),
		)
		return RunResult{}, false, nil
	}
	modelCall := modelCalls[0]
	session.recordToolCallSignature(modelCall.Name, modelCall.Arguments)
	session.turns = append(session.turns, assistantToolCallTurn("", []outbound.ModelToolCall{modelCall}))

	startedAt := r.clock()
	call := outbound.ToolCall{
		RunID:     session.request.RunID,
		Context:   session.request.Context,
		Name:      modelCall.Name,
		Arguments: modelCall.Arguments,
	}
	result, err := r.tools.RunTool(ctx, call)
	endedAt := r.clock()
	if err != nil {
		session.recordToolCall(modelCall.Name, modelCall.Arguments, "tool execution failed", true, startedAt, endedAt)

		return session.partialResult(), false, newAgentError(ErrorCodeToolExecutionFailed, "", err)
	}
	session.toolCallCount++
	session.recordToolCall(modelCall.Name, modelCall.Arguments, result.Content, result.IsError, startedAt, endedAt)

	session.turns = append(session.turns, toolResultTurn([]outbound.ModelPart{{
		Kind:       outbound.ModelPartKindToolResult,
		Text:       buildToolObservation(modelCall.Name, modelCall.Arguments, result),
		ToolCallID: modelCall.ID,
		ToolName:   modelCall.Name,
		IsError:    result.IsError,
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
		if session.toolCallWasUsed(call.Name, call.Arguments) {
			return handleRepeatedToolCall(session, turn, call.Name, call.Arguments)
		}
	}
	if repeatedCall, ok := repeatedToolCall(normalized); ok {
		return handleRepeatedToolCall(session, turn, repeatedCall.Name, repeatedCall.Arguments)
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
		session.recordToolCallSignature(call.Name, call.Arguments)
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

func repeatedToolCall(calls []outbound.ModelToolCall) (outbound.ModelToolCall, bool) {
	seen := make(map[string]outbound.ModelToolCall, len(calls))
	for _, call := range calls {
		signature := toolCallSignature(call.Name, call.Arguments)
		if repeated, ok := seen[signature]; ok {
			return repeated, true
		}
		seen[signature] = call
	}

	return outbound.ModelToolCall{}, false
}

func handleUnavailableNativeToolCall(
	session *runSession,
	turn modelTurnContext,
	call outbound.ModelToolCall,
) (RunResult, bool, error) {
	message := "tool is not available: " + call.Name
	correctionMessage := buildUnavailableToolCorrectionMessage(unavailableToolCorrectionRequest{
		RequestedTool:  call.Name,
		AvailableTools: session.availableToolNames,
	})
	session.recordTurn(TurnRecord{
		Index:             turn.index,
		Status:            run.AgentTurnStatusInvalidToolCall,
		ActionType:        run.AgentTurnActionTypeToolCall,
		ToolName:          call.Name,
		Message:           message,
		RequestMessages:   turn.requestMessages,
		RawResponse:       turn.content,
		ResponseFormat:    turn.responseFormat,
		CorrectionMessage: correctionMessage,
		ResponsePreview:   previewModelResponse(turn.content),
		StartedAt:         turn.startedAt,
		EndedAt:           turn.endedAt,
	})
	session.turns = append(session.turns,
		assistantAttemptTurn(turn.content),
		runtimeTurn(outbound.ModelPartKindToolCorrection, correctionMessage),
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
	correctionMessage := buildPlaceholderToolArgumentCorrectionMessage(placeholderToolArgumentCorrectionRequest{
		Placeholders:    placeholders,
		AvailableTools:  session.availableToolNames,
		UploadedFileIDs: uploadedFilePaths(session.request.Task),
	})
	session.recordTurn(TurnRecord{
		Index:             turn.index,
		Status:            run.AgentTurnStatusInvalidToolCall,
		ActionType:        run.AgentTurnActionTypeToolCall,
		ToolName:          call.Name,
		Message:           message,
		RequestMessages:   turn.requestMessages,
		RawResponse:       turn.content,
		ResponseFormat:    turn.responseFormat,
		CorrectionMessage: correctionMessage,
		ResponsePreview:   previewModelResponse(turn.content),
		StartedAt:         turn.startedAt,
		EndedAt:           turn.endedAt,
	})
	session.turns = append(session.turns,
		assistantAttemptTurn(turn.content),
		runtimeTurn(outbound.ModelPartKindToolCorrection, correctionMessage),
	)

	return RunResult{}, false, nil
}
