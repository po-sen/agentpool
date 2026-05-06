package inbound

import (
	"time"
)

// RunView is the application-level output contract for run use cases.
type RunView struct {
	ID                 string
	Status             string
	Task               TaskView
	Result             RunResultView
	FailureReason      string
	FailureCode        string
	FailureMessage     string
	Steps              []StepView
	ToolCalls          []ToolCallView
	AgentTurns         []AgentTurnView
	Artifacts          []ArtifactView
	AgentSystemPrompt  string
	AgentPromptVersion string
	AgentPromptSHA256  string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// RunResultView is the application-level representation of successful run output.
type RunResultView struct {
	Summary string
}

// TaskView is the application-level representation of submitted task data.
type TaskView struct {
	ProjectID     string
	Prompt        string
	RepositoryURL string
	Branch        string
	Attachments   []AttachmentView
}

// AttachmentView is attachment metadata exposed by application use cases.
type AttachmentView struct {
	Filename  string
	MediaType string
	SizeBytes int64
}

// ArtifactView is artifact metadata exposed by application use cases.
type ArtifactView struct {
	Path      string
	MediaType string
	SizeBytes int64
}

// ArtifactContentView is one artifact's metadata and content.
type ArtifactContentView struct {
	Path      string
	MediaType string
	Content   []byte
	SizeBytes int64
}

// ToolCallView is the application-level representation of a tool execution.
type ToolCallView struct {
	Name      string
	Arguments map[string]string
	Result    string
	IsError   bool
	StartedAt time.Time
	EndedAt   time.Time
}

// AgentTurnView is the application-level representation of one model-loop diagnostic turn.
type AgentTurnView struct {
	Index             int
	Status            string
	ActionType        string
	ToolName          string
	Message           string
	RequestMessages   []AgentTurnMessageView
	RawResponse       string
	ResponseFormat    string
	ProtocolErrorCode string
	CorrectionMessage string
	ResponsePreview   string
	StartedAt         time.Time
	EndedAt           time.Time
}

// AgentTurnMessageView is the application-level representation of one model request message.
type AgentTurnMessageView struct {
	Role       string
	Kind       string
	Content    string
	ToolCallID string
	ToolName   string
}

// StepView is the application-level representation of a run step.
type StepView struct {
	Name      string
	Status    string
	Message   string
	StartedAt time.Time
	EndedAt   *time.Time
}
