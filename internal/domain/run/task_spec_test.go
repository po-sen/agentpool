package run

import (
	"errors"
	"strings"
	"testing"
)

func TestTaskSpecValidateAcceptsMinimalPrompt(t *testing.T) {
	task := TaskSpec{Prompt: "do work"}

	if err := task.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestTaskSpecValidateTreatsEmptyWorkspaceAsNone(t *testing.T) {
	task := TaskSpec{Prompt: "do work"}

	if err := task.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	if task.Workspace.EffectiveType() != WorkspaceSourceNone {
		t.Fatalf("EffectiveType() = %q, want %q", task.Workspace.EffectiveType(), WorkspaceSourceNone)
	}
}

func TestTaskSpecValidateAcceptsConfiguredWorkspace(t *testing.T) {
	task := TaskSpec{
		Prompt:    "do work",
		Workspace: WorkspaceSource{Type: WorkspaceSourceConfigured},
	}

	if err := task.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestTaskSpecValidateRejectsUnknownWorkspace(t *testing.T) {
	task := TaskSpec{
		Prompt:    "do work",
		Workspace: WorkspaceSource{Type: "snapshot"},
	}

	err := task.Validate()
	if !errors.Is(err, ErrUnknownWorkspaceSource) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrUnknownWorkspaceSource)
	}
}

func TestTaskSpecValidateRejectsEmptyPrompt(t *testing.T) {
	task := TaskSpec{Prompt: "   "}

	err := task.Validate()
	if !errors.Is(err, ErrEmptyPrompt) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrEmptyPrompt)
	}
}

func TestTaskSpecValidateRejectsPromptTooLong(t *testing.T) {
	task := TaskSpec{Prompt: strings.Repeat("a", MaxPromptLength+1)}

	err := task.Validate()
	if !errors.Is(err, ErrPromptTooLong) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrPromptTooLong)
	}
}
