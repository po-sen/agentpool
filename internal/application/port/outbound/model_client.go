package outbound

import (
	"context"

	"github.com/po-sen/agentpool/internal/domain/run"
)

// ModelRole identifies a provider-neutral conversation role.
type ModelRole string

const (
	// ModelRoleUser identifies user-provided task/context turns.
	ModelRoleUser ModelRole = "user"
	// ModelRoleRuntime identifies AgentPool runtime instructions that are not user-authored.
	ModelRoleRuntime ModelRole = "runtime"
	// ModelRoleAssistant identifies assistant/model response turns.
	ModelRoleAssistant ModelRole = "assistant"
	// ModelRoleTool identifies tool execution results returned to the model.
	ModelRoleTool ModelRole = "tool"
)

// ModelPartKind identifies the provider-neutral purpose of a model turn part.
type ModelPartKind string

const (
	// ModelPartKindTaskPrompt contains the original user task prompt.
	ModelPartKindTaskPrompt ModelPartKind = "task_prompt"
	// ModelPartKindWorkspaceContext contains run workspace input metadata.
	ModelPartKindWorkspaceContext ModelPartKind = "workspace_context"
	// ModelPartKindAssistantAttempt contains a previous assistant response that failed validation.
	ModelPartKindAssistantAttempt ModelPartKind = "assistant_attempt"
	// ModelPartKindAssistantResponse contains a previous accepted assistant action.
	ModelPartKindAssistantResponse ModelPartKind = "assistant_response"
	// ModelPartKindToolCall contains a previous accepted assistant native tool call.
	ModelPartKindToolCall ModelPartKind = "tool_call"
	// ModelPartKindProtocolCorrection contains protocol repair instructions.
	ModelPartKindProtocolCorrection ModelPartKind = "protocol_correction"
	// ModelPartKindToolCorrection contains tool-call repair instructions.
	ModelPartKindToolCorrection ModelPartKind = "tool_correction"
	// ModelPartKindToolResult contains a tool observation returned to the model.
	ModelPartKindToolResult ModelPartKind = "tool_result"
)

// ModelPart is a provider-neutral content part within a model turn.
type ModelPart struct {
	Kind          ModelPartKind
	Text          string
	ToolCallID    string
	ToolName      string
	ToolArguments map[string]string
	IsError       bool
}

// ModelTurn is a provider-neutral conversation turn.
type ModelTurn struct {
	Role  ModelRole
	Parts []ModelPart
}

// ModelRequest describes a provider-neutral model generation request.
type ModelRequest struct {
	RunID        run.RunID
	Instructions string
	Turns        []ModelTurn
	Tools        []ToolDefinition
}

// ModelResponse contains generated model content.
type ModelResponse struct {
	Content         string
	ToolCalls       []ModelToolCall
	RequestMessages []ModelRequestMessage
}

// ModelRequestMessage records the provider-facing request messages sent to the model.
type ModelRequestMessage struct {
	Role       string
	Kind       string
	Content    string
	ToolCallID string
	ToolName   string
}

// ModelToolCall contains a provider-neutral native tool call returned by a model.
type ModelToolCall struct {
	ID        string
	Name      string
	Arguments map[string]string
}

// ModelClient generates content from provider-neutral model requests.
type ModelClient interface {
	Generate(context.Context, ModelRequest) (ModelResponse, error)
}
