package agent_test

import (
	"context"
	"errors"
	"testing"

	"github.com/po-sen/agentpool/internal/application/agent"
	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestRunnerCallsModelClient(t *testing.T) {
	model := &recordingModelClient{
		response: outbound.ModelResponse{Content: "done"},
	}
	runner := agent.NewRunner(model)

	result, err := runner.Run(context.Background(), agent.RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "do work"},
	})
	if err != nil {
		t.Fatalf("run agent: %v", err)
	}
	if result.Summary != "done" {
		t.Fatalf("summary = %q, want %q", result.Summary, "done")
	}
	if model.request.RunID != "run_test" {
		t.Fatalf("model RunID = %s, want run_test", model.request.RunID)
	}
	if len(model.request.Messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(model.request.Messages))
	}
	if model.request.Messages[0] != (outbound.ModelMessage{Role: "user", Content: "do work"}) {
		t.Fatalf("message = %#v, want user prompt", model.request.Messages[0])
	}
}

func TestRunnerPropagatesModelErrors(t *testing.T) {
	model := &recordingModelClient{err: errModelFailed}
	runner := agent.NewRunner(model)

	_, err := runner.Run(context.Background(), agent.RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "do work"},
	})
	if !errors.Is(err, errModelFailed) {
		t.Fatalf("Run() error = %v, want %v", err, errModelFailed)
	}
}

var errModelFailed = errors.New("model failed")

type recordingModelClient struct {
	request  outbound.ModelRequest
	response outbound.ModelResponse
	err      error
}

func (c *recordingModelClient) Generate(_ context.Context, request outbound.ModelRequest) (outbound.ModelResponse, error) {
	c.request = request

	return c.response, c.err
}
