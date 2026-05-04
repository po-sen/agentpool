package httpapi

import (
	"time"

	"github.com/po-sen/agentpool/internal/application/port/inbound"
)

type createRunRequest struct {
	ProjectID     string           `json:"project_id,omitempty"`
	Prompt        string           `json:"prompt"`
	RepositoryURL string           `json:"repository_url,omitempty"`
	Branch        string           `json:"branch,omitempty"`
	Workspace     workspaceRequest `json:"workspace,omitempty"`
}

type workspaceRequest struct {
	Type       string `json:"type,omitempty"`
	SnapshotID string `json:"snapshot_id,omitempty"`
}

type runResponse struct {
	ID               string                    `json:"id"`
	Status           string                    `json:"status"`
	Task             taskResponse              `json:"task"`
	Result           *runResultResponse        `json:"result,omitempty"`
	FailureReason    string                    `json:"failure_reason,omitempty"`
	WorkspaceChanges []workspaceChangeResponse `json:"workspace_changes,omitempty"`
	Steps            []stepResponse            `json:"steps"`
	CreatedAt        time.Time                 `json:"created_at"`
	UpdatedAt        time.Time                 `json:"updated_at"`
}

type runResultResponse struct {
	Summary string `json:"summary,omitempty"`
}

type taskResponse struct {
	ProjectID     string             `json:"project_id,omitempty"`
	Prompt        string             `json:"prompt"`
	RepositoryURL string             `json:"repository_url,omitempty"`
	Branch        string             `json:"branch,omitempty"`
	Workspace     *workspaceResponse `json:"workspace,omitempty"`
}

type workspaceResponse struct {
	Type       string `json:"type,omitempty"`
	SnapshotID string `json:"snapshot_id,omitempty"`
}

type workspaceChangeResponse struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

type stepResponse struct {
	Name      string     `json:"name"`
	Status    string     `json:"status"`
	Message   string     `json:"message,omitempty"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
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
	changes := make([]workspaceChangeResponse, 0, len(item.WorkspaceChanges))
	for _, change := range item.WorkspaceChanges {
		changes = append(changes, workspaceChangeResponse{
			Path:   change.Path,
			Status: change.Status,
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
		},
		FailureReason:    item.FailureReason,
		WorkspaceChanges: changes,
		Steps:            steps,
		CreatedAt:        item.CreatedAt,
		UpdatedAt:        item.UpdatedAt,
	}
	if item.Result.Summary != "" {
		response.Result = &runResultResponse{Summary: item.Result.Summary}
	}
	if item.Task.Workspace.Type != "" && item.Task.Workspace.Type != "none" {
		response.Task.Workspace = &workspaceResponse{
			Type:       item.Task.Workspace.Type,
			SnapshotID: item.Task.Workspace.SnapshotID,
		}
	}

	return response
}
