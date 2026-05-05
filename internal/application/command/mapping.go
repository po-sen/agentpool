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
		FailureReason: item.FailureReason,
		Steps:         steps,
		ToolCalls:     toolCalls,
		CreatedAt:     item.CreatedAt,
		UpdatedAt:     item.UpdatedAt,
	}
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
