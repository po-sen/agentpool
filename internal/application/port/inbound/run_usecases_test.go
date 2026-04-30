package inbound_test

import (
	"context"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/inbound"
)

func TestRunUseCaseContractsAcceptApplicationDTOs(t *testing.T) {
	var create inbound.CreateRunUseCase = fakeCreateRunUseCase{}
	var get inbound.GetRunUseCase = fakeGetRunUseCase{}
	var list inbound.ListRunsUseCase = fakeListRunsUseCase{}
	var approve inbound.ApproveRunUseCase = fakeApproveRunUseCase{}
	var cancel inbound.CancelRunUseCase = fakeCancelRunUseCase{}

	ctx := context.Background()
	if _, err := create.CreateRun(ctx, inbound.CreateRunCommand{Prompt: "do work"}); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	if _, err := get.GetRun(ctx, inbound.GetRunQuery{RunID: "run_test"}); err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if _, err := list.ListRuns(ctx); err != nil {
		t.Fatalf("ListRuns() error = %v", err)
	}
	if _, err := approve.ApproveRun(ctx, inbound.ApproveRunCommand{RunID: "run_test", Decision: "approved"}); err != nil {
		t.Fatalf("ApproveRun() error = %v", err)
	}
	if _, err := cancel.CancelRun(ctx, inbound.CancelRunCommand{RunID: "run_test"}); err != nil {
		t.Fatalf("CancelRun() error = %v", err)
	}
}

type fakeCreateRunUseCase struct{}

func (fakeCreateRunUseCase) CreateRun(context.Context, inbound.CreateRunCommand) (inbound.RunView, error) {
	return inbound.RunView{ID: "run_test"}, nil
}

type fakeGetRunUseCase struct{}

func (fakeGetRunUseCase) GetRun(context.Context, inbound.GetRunQuery) (inbound.RunView, error) {
	return inbound.RunView{ID: "run_test"}, nil
}

type fakeListRunsUseCase struct{}

func (fakeListRunsUseCase) ListRuns(context.Context) ([]inbound.RunView, error) {
	return []inbound.RunView{{ID: "run_test"}}, nil
}

type fakeApproveRunUseCase struct{}

func (fakeApproveRunUseCase) ApproveRun(context.Context, inbound.ApproveRunCommand) (inbound.RunView, error) {
	return inbound.RunView{ID: "run_test"}, nil
}

type fakeCancelRunUseCase struct{}

func (fakeCancelRunUseCase) CancelRun(context.Context, inbound.CancelRunCommand) (inbound.RunView, error) {
	return inbound.RunView{ID: "run_test"}, nil
}
