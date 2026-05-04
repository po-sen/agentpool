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

func TestTaskSpecValidateAcceptsTextAttachments(t *testing.T) {
	task := TaskSpec{
		Prompt: "do work",
		Attachments: []TaskAttachment{
			textAttachment("README.md", "# Demo\n"),
			textAttachment("internal/main.go", "package main\n"),
		},
	}

	if err := task.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestTaskSpecValidateRejectsTooManyAttachments(t *testing.T) {
	task := TaskSpec{Prompt: "do work"}
	for i := 0; i < MaxAttachmentCount+1; i++ {
		task.Attachments = append(task.Attachments, textAttachment("file"+string(rune('a'+i))+".txt", "ok"))
	}

	err := task.Validate()
	if !errors.Is(err, ErrTooManyAttachments) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrTooManyAttachments)
	}
}

func TestTaskSpecValidateRejectsUnsafeAttachmentFilenames(t *testing.T) {
	tests := []string{
		"",
		" ",
		"/tmp/file.txt",
		"../file.txt",
		"dir/../file.txt",
		"dir//file.txt",
		"dir/./file.txt",
		"dir/",
		`dir\file.txt`,
		"C:/file.txt",
		" file.txt",
		"file.txt ",
	}

	for _, filename := range tests {
		t.Run(filename, func(t *testing.T) {
			err := TaskSpec{
				Prompt:      "do work",
				Attachments: []TaskAttachment{textAttachment(filename, "ok")},
			}.Validate()
			if !errors.Is(err, ErrUnsafeAttachmentFilename) && !errors.Is(err, ErrMissingAttachmentFilename) {
				t.Fatalf("Validate() error = %v, want unsafe or missing filename", err)
			}
		})
	}
}

func TestTaskSpecValidateRejectsUnsupportedAttachmentExtension(t *testing.T) {
	err := TaskSpec{
		Prompt:      "do work",
		Attachments: []TaskAttachment{textAttachment("document.pdf", "%PDF")},
	}.Validate()
	if !errors.Is(err, ErrUnsupportedAttachmentType) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrUnsupportedAttachmentType)
	}
}

func TestTaskSpecValidateRejectsLargeAttachment(t *testing.T) {
	err := TaskSpec{
		Prompt: "do work",
		Attachments: []TaskAttachment{
			textAttachment("large.txt", strings.Repeat("a", int(MaxAttachmentSizeBytes)+1)),
		},
	}.Validate()
	if !errors.Is(err, ErrAttachmentTooLarge) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrAttachmentTooLarge)
	}
}

func TestTaskSpecValidateRejectsTotalAttachmentSize(t *testing.T) {
	task := TaskSpec{Prompt: "do work"}
	for _, filename := range []string{"a.txt", "b.txt", "c.txt", "d.txt", "e.txt", "f.txt"} {
		task.Attachments = append(task.Attachments, textAttachment(filename, strings.Repeat("a", int(MaxAttachmentSizeBytes))))
	}

	err := task.Validate()
	if !errors.Is(err, ErrAttachmentsTooLarge) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrAttachmentsTooLarge)
	}
}

func TestTaskSpecValidateRejectsBinaryAttachment(t *testing.T) {
	err := TaskSpec{
		Prompt: "do work",
		Attachments: []TaskAttachment{
			{
				Filename:  "binary.txt",
				MediaType: "text/plain",
				Content:   []byte{0xff, 0xfe},
				SizeBytes: 2,
			},
		},
	}.Validate()
	if !errors.Is(err, ErrAttachmentNotText) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrAttachmentNotText)
	}
}

func TestTaskSpecValidateRejectsAttachmentSizeMismatch(t *testing.T) {
	err := TaskSpec{
		Prompt: "do work",
		Attachments: []TaskAttachment{
			{
				Filename:  "README.md",
				MediaType: "text/markdown",
				Content:   []byte("ok"),
				SizeBytes: 3,
			},
		},
	}.Validate()
	if !errors.Is(err, ErrAttachmentSizeMismatch) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrAttachmentSizeMismatch)
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

func textAttachment(filename string, content string) TaskAttachment {
	return TaskAttachment{
		Filename:  filename,
		MediaType: "text/plain",
		Content:   []byte(content),
		SizeBytes: int64(len(content)),
	}
}
