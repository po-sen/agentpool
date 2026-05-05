package query

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/po-sen/agentpool/internal/application/port/inbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestRunArtifactsHandlerListsArtifacts(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	item := saveRun(ctx, t, repo, "run_test", "do work", time.Unix(100, 0).UTC())
	item.RecordArtifacts(time.Unix(101, 0).UTC(), []run.Artifact{{
		Path:      "report.md",
		MediaType: "text/markdown",
		Content:   []byte("# Report\n"),
		SizeBytes: 9,
	}})
	if err := repo.Save(ctx, item); err != nil {
		t.Fatalf("save run artifacts: %v", err)
	}
	handler := NewRunArtifactsHandler(repo)

	artifacts, err := handler.ListRunArtifacts(ctx, inbound.GetRunArtifactsQuery{RunID: "run_test"})
	if err != nil {
		t.Fatalf("list run artifacts: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("len(artifacts) = %d, want 1", len(artifacts))
	}
	if artifacts[0].Path != "report.md" || artifacts[0].SizeBytes != 9 {
		t.Fatalf("artifact metadata = %#v, want report metadata", artifacts[0])
	}
}

func TestRunArtifactsHandlerGetsArtifactContent(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	item := saveRun(ctx, t, repo, "run_test", "do work", time.Unix(100, 0).UTC())
	item.RecordArtifacts(time.Unix(101, 0).UTC(), []run.Artifact{{
		Path:      "report.md",
		MediaType: "text/markdown",
		Content:   []byte("# Report\n"),
		SizeBytes: 9,
	}})
	if err := repo.Save(ctx, item); err != nil {
		t.Fatalf("save run artifacts: %v", err)
	}
	handler := NewRunArtifactsHandler(repo)

	artifact, err := handler.GetRunArtifact(ctx, inbound.GetRunArtifactQuery{RunID: "run_test", Path: "report.md"})
	if err != nil {
		t.Fatalf("get run artifact: %v", err)
	}
	if string(artifact.Content) != "# Report\n" {
		t.Fatalf("artifact content = %q, want report", string(artifact.Content))
	}
	artifact.Content[0] = 'X'

	artifact, err = handler.GetRunArtifact(ctx, inbound.GetRunArtifactQuery{RunID: "run_test", Path: "report.md"})
	if err != nil {
		t.Fatalf("get run artifact again: %v", err)
	}
	if string(artifact.Content) != "# Report\n" {
		t.Fatalf("artifact content after mutation = %q, want detached content", string(artifact.Content))
	}
}

func TestRunArtifactsHandlerRejectsUnsafePath(t *testing.T) {
	handler := NewRunArtifactsHandler(newFakeRunRepository())

	_, err := handler.GetRunArtifact(context.Background(), inbound.GetRunArtifactQuery{
		RunID: "run_test",
		Path:  "../secret.txt",
	})
	if !errors.Is(err, inbound.ErrInvalidInput) {
		t.Fatalf("GetRunArtifact() error = %v, want %v", err, inbound.ErrInvalidInput)
	}
}

func TestRunArtifactsHandlerReturnsArtifactNotFound(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	saveRun(ctx, t, repo, "run_test", "do work", time.Unix(100, 0).UTC())
	handler := NewRunArtifactsHandler(repo)

	_, err := handler.GetRunArtifact(ctx, inbound.GetRunArtifactQuery{RunID: "run_test", Path: "missing.txt"})
	if !errors.Is(err, inbound.ErrArtifactNotFound) {
		t.Fatalf("GetRunArtifact() error = %v, want %v", err, inbound.ErrArtifactNotFound)
	}
}
