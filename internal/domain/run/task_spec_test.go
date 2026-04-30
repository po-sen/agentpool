package run_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestTaskSpecValidateAcceptsMinimalPrompt(t *testing.T) {
	task := run.TaskSpec{Prompt: "do work"}

	if err := task.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestTaskSpecValidateRejectsEmptyPrompt(t *testing.T) {
	task := run.TaskSpec{Prompt: "   "}

	err := task.Validate()
	if !errors.Is(err, run.ErrEmptyPrompt) {
		t.Fatalf("Validate() error = %v, want %v", err, run.ErrEmptyPrompt)
	}
}

func TestTaskSpecValidateRejectsPromptTooLong(t *testing.T) {
	task := run.TaskSpec{Prompt: strings.Repeat("a", run.MaxPromptLength+1)}

	err := task.Validate()
	if !errors.Is(err, run.ErrPromptTooLong) {
		t.Fatalf("Validate() error = %v, want %v", err, run.ErrPromptTooLong)
	}
}
