package run

import (
	"testing"
	"time"
)

func TestRunRecordArtifactsStoresDetachedCopies(t *testing.T) {
	item := mustNewRun(t)
	now := time.Unix(200, 0).UTC()
	content := []byte("report\n")

	item.RecordArtifacts(now, []Artifact{{
		Path:      "report.md",
		MediaType: "text/markdown",
		Content:   content,
		SizeBytes: int64(len(content)),
	}})
	content[0] = 'X'

	if len(item.Artifacts) != 1 {
		t.Fatalf("len(Artifacts) = %d, want 1", len(item.Artifacts))
	}
	if string(item.Artifacts[0].Content) != "report\n" {
		t.Fatalf("artifact content = %q, want detached content", string(item.Artifacts[0].Content))
	}
	if !item.UpdatedAt.Equal(now) {
		t.Fatalf("UpdatedAt = %v, want %v", item.UpdatedAt, now)
	}

	clone := item.Clone()
	clone.Artifacts[0].Content[0] = 'X'
	if string(item.Artifacts[0].Content) != "report\n" {
		t.Fatalf("artifact content after clone mutation = %q, want detached original", string(item.Artifacts[0].Content))
	}
}

func TestRunArtifactByPathReturnsDetachedArtifact(t *testing.T) {
	item := mustNewRun(t)
	item.RecordArtifacts(time.Unix(200, 0).UTC(), []Artifact{{
		Path:    "report.md",
		Content: []byte("report\n"),
	}})

	artifact, ok := item.ArtifactByPath("report.md")
	if !ok {
		t.Fatal("ArtifactByPath ok = false, want true")
	}
	artifact.Content[0] = 'X'
	if string(item.Artifacts[0].Content) != "report\n" {
		t.Fatalf("stored artifact content = %q, want detached original", string(item.Artifacts[0].Content))
	}
}

func TestValidateArtifactPathRejectsUnsafePaths(t *testing.T) {
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

	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			if err := ValidateArtifactPath(path); err == nil {
				t.Fatal("ValidateArtifactPath() error = nil, want error")
			}
		})
	}
}

func TestValidateArtifactPathAcceptsRelativePath(t *testing.T) {
	if err := ValidateArtifactPath("reports/report.md"); err != nil {
		t.Fatalf("ValidateArtifactPath() error = %v, want nil", err)
	}
}

func mustNewRun(t *testing.T) *Run {
	t.Helper()

	item, err := New("run_test", TaskSpec{Prompt: "do work"}, time.Unix(100, 0).UTC())
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	if item == nil {
		t.Fatal("new run returned nil")
	}

	return item
}
