package agent

import (
	"reflect"
	"testing"

	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestUploadedFilePathsReturnsAttachmentFilenames(t *testing.T) {
	got := uploadedFilePaths(run.TaskSpec{
		Attachments: []run.TaskAttachment{
			{Filename: "README.md"},
			{Filename: "docs/spec.txt"},
		},
	})
	want := []string{"README.md", "docs/spec.txt"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("uploadedFilePaths() = %#v, want %#v", got, want)
	}
}
