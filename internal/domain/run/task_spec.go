package run

import (
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// MaxPromptLength is the maximum accepted task prompt length in characters.
const MaxPromptLength = 8000

const (
	// MaxAttachmentCount is the maximum number of files accepted for one task.
	MaxAttachmentCount = 500
	// MaxAttachmentSizeBytes is the maximum size accepted for one uploaded file.
	MaxAttachmentSizeBytes int64 = 50 << 20
	// MaxTotalAttachmentSizeBytes is the maximum total size accepted for all uploaded files.
	MaxTotalAttachmentSizeBytes int64 = 200 << 20
)

var textAttachmentExtensions = map[string]struct{}{
	".csv":  {},
	".css":  {},
	".go":   {},
	".html": {},
	".js":   {},
	".json": {},
	".jsx":  {},
	".md":   {},
	".mod":  {},
	".py":   {},
	".sh":   {},
	".sum":  {},
	".toml": {},
	".ts":   {},
	".tsx":  {},
	".txt":  {},
	".xml":  {},
	".yaml": {},
	".yml":  {},
}

var binaryAttachmentExtensions = map[string]struct{}{
	".bmp":  {},
	".doc":  {},
	".docx": {},
	".gif":  {},
	".heic": {},
	".jpeg": {},
	".jpg":  {},
	".odp":  {},
	".ods":  {},
	".odt":  {},
	".pdf":  {},
	".png":  {},
	".ppt":  {},
	".pptx": {},
	".rtf":  {},
	".tif":  {},
	".tiff": {},
	".webp": {},
	".xls":  {},
	".xlsx": {},
}

var textAttachmentBasenames = map[string]struct{}{
	".dockerignore": {},
	".env.example":  {},
	".gitignore":    {},
	"dockerfile":    {},
	"makefile":      {},
}

// TaskSpec describes the work requested by a run submitter.
type TaskSpec struct {
	ProjectID     string
	Prompt        string
	RepositoryURL string
	Branch        string
	Attachments   []TaskAttachment
}

// TaskAttachment describes one already-authorized file supplied with a run.
type TaskAttachment struct {
	Filename  string
	MediaType string
	Content   []byte
	SizeBytes int64
}

// Validate checks the minimal task fields required to queue a run.
func (s TaskSpec) Validate() error {
	if strings.TrimSpace(s.Prompt) == "" {
		return ErrEmptyPrompt
	}
	if utf8.RuneCountInString(s.Prompt) > MaxPromptLength {
		return ErrPromptTooLong
	}

	return s.validateAttachments()
}

// Clone returns a detached copy of the task spec.
func (s TaskSpec) Clone() TaskSpec {
	clone := s
	if len(s.Attachments) == 0 {
		return clone
	}

	clone.Attachments = make([]TaskAttachment, 0, len(s.Attachments))
	for _, attachment := range s.Attachments {
		item := attachment
		if attachment.Content != nil {
			item.Content = append([]byte(nil), attachment.Content...)
		}
		clone.Attachments = append(clone.Attachments, item)
	}

	return clone
}

func (s TaskSpec) validateAttachments() error {
	if len(s.Attachments) > MaxAttachmentCount {
		return ErrTooManyAttachments
	}

	var totalSize int64
	for _, attachment := range s.Attachments {
		if err := attachment.Validate(); err != nil {
			return err
		}
		totalSize += attachment.effectiveSize()
		if totalSize > MaxTotalAttachmentSizeBytes {
			return ErrAttachmentsTooLarge
		}
	}

	return nil
}

// Validate checks whether the attachment is safe for the runtime workspace.
func (a TaskAttachment) Validate() error {
	if err := validateAttachmentFilename(a.Filename); err != nil {
		return err
	}
	if !supportedAttachmentName(a.Filename) {
		return ErrUnsupportedAttachmentType
	}
	if a.SizeBytes < 0 {
		return ErrAttachmentSizeMismatch
	}
	if a.SizeBytes != 0 && a.SizeBytes != int64(len(a.Content)) {
		return ErrAttachmentSizeMismatch
	}
	if a.effectiveSize() > MaxAttachmentSizeBytes {
		return ErrAttachmentTooLarge
	}
	if attachmentRequiresUTF8(a.Filename) && !utf8.Valid(a.Content) {
		return ErrAttachmentNotText
	}

	return nil
}

func supportedAttachmentName(filename string) bool {
	base := strings.ToLower(path.Base(filename))
	if _, ok := textAttachmentBasenames[base]; ok {
		return true
	}
	extension := strings.ToLower(path.Ext(filename))
	if _, ok := textAttachmentExtensions[extension]; ok {
		return true
	}
	_, ok := binaryAttachmentExtensions[extension]

	return ok
}

func attachmentRequiresUTF8(filename string) bool {
	base := strings.ToLower(path.Base(filename))
	if _, ok := textAttachmentBasenames[base]; ok {
		return true
	}
	_, ok := textAttachmentExtensions[strings.ToLower(path.Ext(filename))]

	return ok
}

func (a TaskAttachment) effectiveSize() int64 {
	if a.SizeBytes > 0 {
		return a.SizeBytes
	}

	return int64(len(a.Content))
}

func validateAttachmentFilename(filename string) error {
	if strings.TrimSpace(filename) == "" {
		return ErrMissingAttachmentFilename
	}
	if filename != strings.TrimSpace(filename) ||
		path.IsAbs(filename) ||
		filepath.IsAbs(filename) ||
		filepath.VolumeName(filename) != "" ||
		strings.Contains(filename, "\\") ||
		strings.Contains(filename, ":") {
		return ErrUnsafeAttachmentFilename
	}

	components := strings.Split(filename, "/")
	for _, component := range components {
		if component == "" || component == "." || component == ".." {
			return ErrUnsafeAttachmentFilename
		}
	}
	if path.Clean(filename) != filename {
		return ErrUnsafeAttachmentFilename
	}

	return nil
}
