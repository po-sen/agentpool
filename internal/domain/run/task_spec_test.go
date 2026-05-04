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
