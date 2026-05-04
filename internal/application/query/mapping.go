package query

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
		CreatedAt:     item.CreatedAt,
		UpdatedAt:     item.UpdatedAt,
	}
}
