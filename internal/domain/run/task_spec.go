package run

import (
	"strings"
	"unicode/utf8"
)

// MaxPromptLength is the maximum accepted task prompt length in characters.
const MaxPromptLength = 8000

// WorkspaceSourceType identifies how a run should receive workspace access.
type WorkspaceSourceType string

const (
	// WorkspaceSourceNone means the run should not receive workspace access.
	WorkspaceSourceNone WorkspaceSourceType = "none"
	// WorkspaceSourceConfigured means the run should use the configured local workspace source.
	WorkspaceSourceConfigured WorkspaceSourceType = "configured"
)

// WorkspaceSource describes the workspace capability requested by a run.
type WorkspaceSource struct {
	Type WorkspaceSourceType
}

// TaskSpec describes the work requested by a run submitter.
type TaskSpec struct {
	ProjectID     string
	Prompt        string
	RepositoryURL string
	Branch        string
	Workspace     WorkspaceSource
}

// Validate checks the minimal task fields required to queue a run.
func (s TaskSpec) Validate() error {
	if strings.TrimSpace(s.Prompt) == "" {
		return ErrEmptyPrompt
	}
	if utf8.RuneCountInString(s.Prompt) > MaxPromptLength {
		return ErrPromptTooLong
	}
	if !s.Workspace.IsValid() {
		return ErrUnknownWorkspaceSource
	}

	return nil
}

// EffectiveType returns the workspace source type after applying defaults.
func (s WorkspaceSource) EffectiveType() WorkspaceSourceType {
	if s.Type == "" {
		return WorkspaceSourceNone
	}

	return s.Type
}

// IsValid reports whether the workspace source is supported.
func (s WorkspaceSource) IsValid() bool {
	switch s.EffectiveType() {
	case WorkspaceSourceNone, WorkspaceSourceConfigured:
		return true
	default:
		return false
	}
}
