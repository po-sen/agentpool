package httpapi

import (
	"time"

	"github.com/po-sen/agentpool/internal/application/port/inbound"
)

type createRunRequest struct {
	ProjectID     string `json:"project_id,omitempty"`
	Prompt        string `json:"prompt"`
	RepositoryURL string `json:"repository_url,omitempty"`
	Branch        string `json:"branch,omitempty"`
}

type runResponse struct {
	ID             string              `json:"id"`
	Status         string              `json:"status"`
	Task           taskResponse        `json:"task"`
	Result         *runResultResponse  `json:"result,omitempty"`
	FailureReason  string              `json:"failure_reason,omitempty"`
	FailureCode    string              `json:"failure_code,omitempty"`
	FailureMessage string              `json:"failure_message,omitempty"`
	Steps          []stepResponse      `json:"steps"`
	ToolCalls      []toolCallResponse  `json:"tool_calls,omitempty"`
	AgentTurns     []agentTurnResponse `json:"agent_turns,omitempty"`
	CreatedAt      time.Time           `json:"created_at"`
	UpdatedAt      time.Time           `json:"updated_at"`
}

type runResultResponse struct {
	Summary string `json:"summary,omitempty"`
}

type taskResponse struct {
	ProjectID     string               `json:"project_id,omitempty"`
	Prompt        string               `json:"prompt"`
	RepositoryURL string               `json:"repository_url,omitempty"`
	Branch        string               `json:"branch,omitempty"`
	Attachments   []attachmentResponse `json:"attachments,omitempty"`
}

type attachmentResponse struct {
	Filename  string `json:"filename"`
	MediaType string `json:"media_type,omitempty"`
	SizeBytes int64  `json:"size_bytes"`
}

type stepResponse struct {
	Name      string     `json:"name"`
	Status    string     `json:"status"`
	Message   string     `json:"message,omitempty"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}

type toolCallResponse struct {
	Name      string            `json:"name"`
	Arguments map[string]string `json:"arguments"`
	Result    string            `json:"result"`
	IsError   bool              `json:"is_error"`
	StartedAt time.Time         `json:"started_at"`
	EndedAt   time.Time         `json:"ended_at"`
}

type agentTurnResponse struct {
	Index           int       `json:"index"`
	Status          string    `json:"status"`
	ActionType      string    `json:"action_type,omitempty"`
	ToolName        string    `json:"tool_name,omitempty"`
	Message         string    `json:"message,omitempty"`
	ResponsePreview string    `json:"response_preview,omitempty"`
	StartedAt       time.Time `json:"started_at"`
	EndedAt         time.Time `json:"ended_at"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func toRunResponse(item inbound.RunView) runResponse {
	steps := make([]stepResponse, 0, len(item.Steps))
	for _, step := range item.Steps {
		steps = append(steps, stepResponse{
			Name:      step.Name,
			Status:    step.Status,
			Message:   step.Message,
			StartedAt: step.StartedAt,
			EndedAt:   step.EndedAt,
		})
	}
	attachments := make([]attachmentResponse, 0, len(item.Task.Attachments))
	for _, attachment := range item.Task.Attachments {
		attachments = append(attachments, attachmentResponse{
			Filename:  attachment.Filename,
			MediaType: attachment.MediaType,
			SizeBytes: attachment.SizeBytes,
		})
	}
	toolCalls := make([]toolCallResponse, 0, len(item.ToolCalls))
	for _, call := range item.ToolCalls {
		toolCalls = append(toolCalls, toolCallResponse{
			Name:      call.Name,
			Arguments: toolCallArgumentsResponse(call.Arguments),
			Result:    call.Result,
			IsError:   call.IsError,
			StartedAt: call.StartedAt,
			EndedAt:   call.EndedAt,
		})
	}
	agentTurns := make([]agentTurnResponse, 0, len(item.AgentTurns))
	for _, turn := range item.AgentTurns {
		agentTurns = append(agentTurns, agentTurnResponse{
			Index:           turn.Index,
			Status:          turn.Status,
			ActionType:      turn.ActionType,
			ToolName:        turn.ToolName,
			Message:         turn.Message,
			ResponsePreview: turn.ResponsePreview,
			StartedAt:       turn.StartedAt,
			EndedAt:         turn.EndedAt,
		})
	}
	response := runResponse{
		ID:     item.ID,
		Status: item.Status,
		Task: taskResponse{
			ProjectID:     item.Task.ProjectID,
			Prompt:        item.Task.Prompt,
			RepositoryURL: item.Task.RepositoryURL,
			Branch:        item.Task.Branch,
			Attachments:   attachments,
		},
		FailureReason:  item.FailureReason,
		FailureCode:    item.FailureCode,
		FailureMessage: item.FailureMessage,
		Steps:          steps,
		ToolCalls:      toolCalls,
		AgentTurns:     agentTurns,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
	if item.Result.Summary != "" {
		response.Result = &runResultResponse{Summary: item.Result.Summary}
	}

	return response
}

func toolCallArgumentsResponse(arguments map[string]string) map[string]string {
	if arguments != nil {
		return arguments
	}

	return map[string]string{}
}
