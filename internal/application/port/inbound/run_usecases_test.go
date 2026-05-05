package inbound

import (
	"context"
	"testing"
)

func TestRunUseCaseContractsAcceptApplicationDTOs(t *testing.T) {
	var create CreateRunUseCase = fakeCreateRunUseCase{}
	var get GetRunUseCase = fakeGetRunUseCase{}
	var list ListRunsUseCase = fakeListRunsUseCase{}
	var listArtifacts ListRunArtifactsUseCase = fakeListRunArtifactsUseCase{}
	var getArtifact GetRunArtifactUseCase = fakeGetRunArtifactUseCase{}
	var approve ApproveRunUseCase = fakeApproveRunUseCase{}
	var cancel CancelRunUseCase = fakeCancelRunUseCase{}

	ctx := context.Background()
	if _, err := create.CreateRun(ctx, CreateRunCommand{
		Prompt: "do work",
		Attachments: []AttachmentInput{
			{Filename: "README.md", MediaType: "text/markdown", Content: []byte("# Demo\n"), SizeBytes: 7},
		},
	}); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	if _, err := get.GetRun(ctx, GetRunQuery{RunID: "run_test"}); err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if _, err := list.ListRuns(ctx); err != nil {
		t.Fatalf("ListRuns() error = %v", err)
	}
	if _, err := listArtifacts.ListRunArtifacts(ctx, GetRunArtifactsQuery{RunID: "run_test"}); err != nil {
		t.Fatalf("ListRunArtifacts() error = %v", err)
	}
	if _, err := getArtifact.GetRunArtifact(ctx, GetRunArtifactQuery{RunID: "run_test", Path: "report.md"}); err != nil {
		t.Fatalf("GetRunArtifact() error = %v", err)
	}
	if _, err := approve.ApproveRun(ctx, ApproveRunCommand{RunID: "run_test", Decision: "approved"}); err != nil {
		t.Fatalf("ApproveRun() error = %v", err)
	}
	if _, err := cancel.CancelRun(ctx, CancelRunCommand{RunID: "run_test"}); err != nil {
		t.Fatalf("CancelRun() error = %v", err)
	}
}

type fakeCreateRunUseCase struct{}

func (fakeCreateRunUseCase) CreateRun(context.Context, CreateRunCommand) (RunView, error) {
	return RunView{ID: "run_test"}, nil
}

type fakeGetRunUseCase struct{}

func (fakeGetRunUseCase) GetRun(context.Context, GetRunQuery) (RunView, error) {
	return RunView{ID: "run_test"}, nil
}

type fakeListRunsUseCase struct{}

func (fakeListRunsUseCase) ListRuns(context.Context) ([]RunView, error) {
	return []RunView{{ID: "run_test"}}, nil
}

type fakeListRunArtifactsUseCase struct{}

func (fakeListRunArtifactsUseCase) ListRunArtifacts(context.Context, GetRunArtifactsQuery) ([]ArtifactView, error) {
	return []ArtifactView{{Path: "report.md"}}, nil
}

type fakeGetRunArtifactUseCase struct{}

func (fakeGetRunArtifactUseCase) GetRunArtifact(context.Context, GetRunArtifactQuery) (ArtifactContentView, error) {
	return ArtifactContentView{Path: "report.md", Content: []byte("# Report\n")}, nil
}

type fakeApproveRunUseCase struct{}

func (fakeApproveRunUseCase) ApproveRun(context.Context, ApproveRunCommand) (RunView, error) {
	return RunView{ID: "run_test"}, nil
}

type fakeCancelRunUseCase struct{}

func (fakeCancelRunUseCase) CancelRun(context.Context, CancelRunCommand) (RunView, error) {
	return RunView{ID: "run_test"}, nil
}
