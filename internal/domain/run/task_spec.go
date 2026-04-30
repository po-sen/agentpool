package run

import (
	"strings"
	"unicode/utf8"
)

// MaxPromptLength is the maximum accepted task prompt length in characters.
const MaxPromptLength = 8000

// TaskSpec describes the work requested by a run submitter.
type TaskSpec struct {
	ProjectID     string
	Prompt        string
	RepositoryURL string
	Branch        string
}

// Validate checks the minimal task fields required to queue a run.
func (s TaskSpec) Validate() error {
	if strings.TrimSpace(s.Prompt) == "" {
		return ErrEmptyPrompt
	}
	if utf8.RuneCountInString(s.Prompt) > MaxPromptLength {
		return ErrPromptTooLong
	}

	return nil
}
