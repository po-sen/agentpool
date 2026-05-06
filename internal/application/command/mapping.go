package command

import (
	"time"

	"github.com/po-sen/agentpool/internal/application/port/inbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

func toRunView(item *run.Run) inbound.RunView {
	steps := make([]inbound.StepView, 0, len(item.Steps))
	for _, step := range item.Steps {
		var endedAt *time.Time
		if !step.EndedAt.IsZero() {
			value := step.EndedAt
			endedAt = &value
		}

		steps = append(steps, inbound.StepView{
			Name:      step.Name,
			Status:    string(step.Status),
			Message:   step.Message,
			StartedAt: step.StartedAt,
			EndedAt:   endedAt,
		})
	}
	attachments := make([]inbound.AttachmentView, 0, len(item.Task.Attachments))
	for _, attachment := range item.Task.Attachments {
		attachments = append(attachments, inbound.AttachmentView{
			Filename:  attachment.Filename,
			MediaType: attachment.MediaType,
			SizeBytes: attachment.SizeBytes,
		})
	}
	toolCalls := make([]inbound.ToolCallView, 0, len(item.ToolCalls))
	for _, call := range item.ToolCalls {
		toolCalls = append(toolCalls, inbound.ToolCallView{
			Name:      call.Name,
			Arguments: copyToolArguments(call.Arguments),
			Result:    call.Result,
			IsError:   call.IsError,
			StartedAt: call.StartedAt,
			EndedAt:   call.EndedAt,
		})
	}
	agentTurns := make([]inbound.AgentTurnView, 0, len(item.AgentTurns))
	for _, turn := range item.AgentTurns {
		agentTurns = append(agentTurns, inbound.AgentTurnView{
			Index:             turn.Index,
			Status:            turn.Status,
			ActionType:        turn.ActionType,
			ToolName:          turn.ToolName,
			Message:           turn.Message,
			RequestMessages:   agentTurnMessageViews(turn.RequestMessages),
			RawResponse:       turn.RawResponse,
			ResponseFormat:    turn.ResponseFormat,
			ProtocolErrorCode: turn.ProtocolErrorCode,
			CorrectionMessage: turn.CorrectionMessage,
			ResponsePreview:   turn.ResponsePreview,
			StartedAt:         turn.StartedAt,
			EndedAt:           turn.EndedAt,
		})
	}
	artifacts := artifactViews(item.Artifacts)

	return inbound.RunView{
		ID:     item.ID.String(),
		Status: string(item.Status),
		Task: inbound.TaskView{
			ProjectID:     item.Task.ProjectID,
			Prompt:        item.Task.Prompt,
			RepositoryURL: item.Task.RepositoryURL,
			Branch:        item.Task.Branch,
			Attachments:   attachments,
		},
		Result: inbound.RunResultView{
			Summary: item.ResultSummary,
		},
		FailureReason:  item.FailureReason,
		FailureCode:    item.FailureCode,
		FailureMessage: item.FailureMessage,
		Steps:          steps,
		ToolCalls:      toolCalls,
		AgentTurns:     agentTurns,
		Artifacts:      artifacts,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
}

func agentTurnMessageViews(messages []run.AgentTurnMessage) []inbound.AgentTurnMessageView {
	if len(messages) == 0 {
		return nil
	}

	views := make([]inbound.AgentTurnMessageView, 0, len(messages))
	for _, message := range messages {
		views = append(views, inbound.AgentTurnMessageView{
			Role:       message.Role,
			Kind:       message.Kind,
			Content:    message.Content,
			ToolCallID: message.ToolCallID,
			ToolName:   message.ToolName,
		})
	}

	return views
}

func artifactViews(artifacts []run.Artifact) []inbound.ArtifactView {
	if len(artifacts) == 0 {
		return nil
	}

	views := make([]inbound.ArtifactView, 0, len(artifacts))
	for _, artifact := range artifacts {
		views = append(views, inbound.ArtifactView{
			Path:      artifact.Path,
			MediaType: artifact.MediaType,
			SizeBytes: artifact.SizeBytes,
		})
	}

	return views
}

func copyToolArguments(arguments map[string]string) map[string]string {
	if len(arguments) == 0 {
		return nil
	}

	copied := make(map[string]string, len(arguments))
	for key, value := range arguments {
		copied[key] = value
	}

	return copied
}
