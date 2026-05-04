package agent_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/po-sen/agentpool/internal/application/agent"
	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestRunnerTreatsNaturalLanguageResponseAsFinalSummary(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{{Content: "done"}},
	}
	tools := newFakeToolRunner()
	runner := agent.NewRunner(model, tools)

	result, err := runner.Run(context.Background(), agent.RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "do work"},
	})
	if err != nil {
		t.Fatalf("run agent: %v", err)
	}
	if result.Summary != "done" {
		t.Fatalf("summary = %q, want done", result.Summary)
	}
	if result.ToolCallCount != 0 {
		t.Fatalf("ToolCallCount = %d, want 0", result.ToolCallCount)
	}
	if model.requests[0].RunID != "run_test" {
		t.Fatalf("model RunID = %s, want run_test", model.requests[0].RunID)
	}
	assertMessage(t, model.requests[0].Messages[0], "system", "Available tools")
	assertMessage(t, model.requests[0].Messages[1], "user", "do work")
}

func TestRunnerReturnsJSONFinalActionSummary(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{{Content: `{"type":"final","summary":"done"}`}},
	}
	runner := agent.NewRunner(model, newFakeToolRunner())

	result, err := runner.Run(context.Background(), agent.RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "do work"},
	})
	if err != nil {
		t.Fatalf("run agent: %v", err)
	}
	if result.Summary != "done" {
		t.Fatalf("summary = %q, want done", result.Summary)
	}
}

func TestRunnerCallsToolAndReturnsFinalSummary(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{
			{Content: `{"type":"tool_call","tool":"echo","arguments":{"text":"hello"}}`},
			{Content: `{"type":"final","summary":"echoed hello"}`},
		},
	}
	tools := newFakeToolRunner()
	runner := agent.NewRunner(model, tools)

	result, err := runner.Run(context.Background(), agent.RunRequest{
		RunID:   "run_test",
		Task:    run.TaskSpec{Prompt: "do work"},
		Sandbox: outbound.Sandbox{ID: "sandbox_test"},
	})
	if err != nil {
		t.Fatalf("run agent: %v", err)
	}
	if result.Summary != "echoed hello" {
		t.Fatalf("summary = %q, want echoed hello", result.Summary)
	}
	if result.ToolCallCount != 1 {
		t.Fatalf("ToolCallCount = %d, want 1", result.ToolCallCount)
	}
	if len(tools.calls) != 1 {
		t.Fatalf("len(tool calls) = %d, want 1", len(tools.calls))
	}
	if tools.calls[0].Name != "echo" {
		t.Fatalf("tool name = %q, want echo", tools.calls[0].Name)
	}
	if tools.calls[0].Arguments["text"] != "hello" {
		t.Fatalf("tool text = %q, want hello", tools.calls[0].Arguments["text"])
	}
	if tools.calls[0].Sandbox.ID != "sandbox_test" {
		t.Fatalf("tool sandbox = %q, want sandbox_test", tools.calls[0].Sandbox.ID)
	}
	if len(model.requests) != 2 {
		t.Fatalf("len(model requests) = %d, want 2", len(model.requests))
	}
	lastMessages := model.requests[1].Messages
	assertMessage(t, lastMessages[len(lastMessages)-2], "assistant", `"type":"tool_call"`)
	assertMessage(t, lastMessages[len(lastMessages)-1], "user", "Tool result for echo:\nhello")
}

func TestRunnerFeedsUnknownToolResultBackToModel(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{
			{Content: `{"type":"tool_call","tool":"missing","arguments":{}}`},
			{Content: `{"type":"final","summary":"handled tool error"}`},
		},
	}
	tools := newFakeToolRunner()
	runner := agent.NewRunner(model, tools)

	result, err := runner.Run(context.Background(), agent.RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "do work"},
	})
	if err != nil {
		t.Fatalf("run agent: %v", err)
	}
	if result.Summary != "handled tool error" {
		t.Fatalf("summary = %q, want handled tool error", result.Summary)
	}
	if result.ToolCallCount != 1 {
		t.Fatalf("ToolCallCount = %d, want 1", result.ToolCallCount)
	}
	lastMessages := model.requests[1].Messages
	assertMessage(t, lastMessages[len(lastMessages)-1], "user", "Tool error for missing:\nunknown tool: missing")
}

func TestRunnerRejectsUnknownJSONActionTypeAndContinues(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{
			{Content: `{"type":"tool_result","result":"hello from tool"}`},
			{Content: `{"type":"final","summary":"corrected final"}`},
		},
	}
	tools := newFakeToolRunner()
	runner := agent.NewRunner(model, tools)

	result, err := runner.Run(context.Background(), agent.RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "do work"},
	})
	if err != nil {
		t.Fatalf("run agent: %v", err)
	}
	if result.Summary != "corrected final" {
		t.Fatalf("summary = %q, want corrected final", result.Summary)
	}
	if strings.Contains(result.Summary, "tool_result") {
		t.Fatalf("summary accepted invalid action: %q", result.Summary)
	}
	if len(model.requests) != 2 {
		t.Fatalf("len(model requests) = %d, want 2", len(model.requests))
	}
	lastMessages := model.requests[1].Messages
	assertMessage(t, lastMessages[len(lastMessages)-2], "assistant", "tool_result")
	assertMessage(t, lastMessages[len(lastMessages)-1], "user", "Protocol error:")
	assertMessage(t, lastMessages[len(lastMessages)-1], "user", "Do not return tool_result.")
	if len(tools.calls) != 0 {
		t.Fatalf("len(tool calls) = %d, want 0", len(tools.calls))
	}
}

func TestRunnerRejectsMultipleJSONObjectsAndContinues(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{
			{Content: `{"type":"tool_call","tool":"echo","arguments":{"text":"hello"}}{"type":"final","summary":"bad"}`},
			{Content: `{"type":"final","summary":"corrected final"}`},
		},
	}
	tools := newFakeToolRunner()
	runner := agent.NewRunner(model, tools)

	result, err := runner.Run(context.Background(), agent.RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "do work"},
	})
	if err != nil {
		t.Fatalf("run agent: %v", err)
	}
	if result.Summary != "corrected final" {
		t.Fatalf("summary = %q, want corrected final", result.Summary)
	}
	if len(tools.calls) != 0 {
		t.Fatalf("len(tool calls) = %d, want 0", len(tools.calls))
	}
	if len(model.requests) != 2 {
		t.Fatalf("len(model requests) = %d, want 2", len(model.requests))
	}
	lastMessages := model.requests[1].Messages
	assertMessage(t, lastMessages[len(lastMessages)-1], "user", "Do not return multiple JSON objects.")
}

func TestRunnerRejectsFencedJSONActionAndContinues(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{
			{Content: "```json\n{\"type\":\"tool_call\",\"tool\":\"echo\",\"arguments\":{\"text\":\"hello\"}}\n```"},
			{Content: `{"type":"final","summary":"corrected final"}`},
		},
	}
	tools := newFakeToolRunner()
	runner := agent.NewRunner(model, tools)

	result, err := runner.Run(context.Background(), agent.RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "do work"},
	})
	if err != nil {
		t.Fatalf("run agent: %v", err)
	}
	if result.Summary != "corrected final" {
		t.Fatalf("summary = %q, want corrected final", result.Summary)
	}
	if len(tools.calls) != 0 {
		t.Fatalf("len(tool calls) = %d, want 0", len(tools.calls))
	}
	if len(model.requests) != 2 {
		t.Fatalf("len(model requests) = %d, want 2", len(model.requests))
	}
	lastMessages := model.requests[1].Messages
	assertMessage(t, lastMessages[len(lastMessages)-2], "assistant", "```json")
	assertMessage(t, lastMessages[len(lastMessages)-1], "user", "Do not use markdown fences.")
}

func TestRunnerRejectsEmptyFinalSummaryAndContinues(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{
			{Content: `{"type":"final","summary":""}`},
			{Content: `{"type":"final","summary":"fixed"}`},
		},
	}
	runner := agent.NewRunner(model, newFakeToolRunner())

	result, err := runner.Run(context.Background(), agent.RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "do work"},
	})
	if err != nil {
		t.Fatalf("run agent: %v", err)
	}
	if result.Summary != "fixed" {
		t.Fatalf("summary = %q, want fixed", result.Summary)
	}
}

func TestRunnerRejectsToolCallWithMissingToolAndContinues(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{
			{Content: `{"type":"tool_call","arguments":{"text":"hello"}}`},
			{Content: `{"type":"final","summary":"fixed"}`},
		},
	}
	tools := newFakeToolRunner()
	runner := agent.NewRunner(model, tools)

	result, err := runner.Run(context.Background(), agent.RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "do work"},
	})
	if err != nil {
		t.Fatalf("run agent: %v", err)
	}
	if result.Summary != "fixed" {
		t.Fatalf("summary = %q, want fixed", result.Summary)
	}
	if len(tools.calls) != 0 {
		t.Fatalf("len(tool calls) = %d, want 0", len(tools.calls))
	}
}

func TestRunnerReturnsErrorWhenMaxTurnsExceeded(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{{Content: `{"type":"tool_call","tool":"echo","arguments":{"text":"hello"}}`}},
	}
	tools := newFakeToolRunner()
	runner := agent.NewRunner(model, tools, agent.WithMaxTurns(1))

	_, err := runner.Run(context.Background(), agent.RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "do work"},
	})
	if err == nil || !strings.Contains(err.Error(), "max turns") {
		t.Fatalf("Run() error = %v, want max turns error", err)
	}
	if len(tools.calls) != 1 {
		t.Fatalf("len(tool calls) = %d, want 1", len(tools.calls))
	}
}

func TestRunnerReturnsErrorWhenProtocolErrorsExceedMaxTurns(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{{Content: `{"type":"tool_result","result":"hello from tool"}`}},
	}
	tools := newFakeToolRunner()
	runner := agent.NewRunner(model, tools, agent.WithMaxTurns(1))

	_, err := runner.Run(context.Background(), agent.RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "do work"},
	})
	if err == nil || !strings.Contains(err.Error(), "max turns") {
		t.Fatalf("Run() error = %v, want max turns error", err)
	}
	if len(tools.calls) != 0 {
		t.Fatalf("len(tool calls) = %d, want 0", len(tools.calls))
	}
}

func TestRunnerPropagatesModelErrors(t *testing.T) {
	model := &recordingModelClient{err: errModelFailed}
	runner := agent.NewRunner(model, newFakeToolRunner())

	_, err := runner.Run(context.Background(), agent.RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "do work"},
	})
	if !errors.Is(err, errModelFailed) {
		t.Fatalf("Run() error = %v, want %v", err, errModelFailed)
	}
}

func TestRunnerPropagatesListToolsErrors(t *testing.T) {
	model := &recordingModelClient{responses: []outbound.ModelResponse{{Content: "done"}}}
	tools := &fakeToolRunner{listErr: errListToolsFailed}
	runner := agent.NewRunner(model, tools)

	_, err := runner.Run(context.Background(), agent.RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "do work"},
	})
	if !errors.Is(err, errListToolsFailed) {
		t.Fatalf("Run() error = %v, want %v", err, errListToolsFailed)
	}
	if len(model.requests) != 0 {
		t.Fatalf("len(model requests) = %d, want 0", len(model.requests))
	}
}

var errModelFailed = errors.New("model failed")
var errListToolsFailed = errors.New("list tools failed")

type recordingModelClient struct {
	requests  []outbound.ModelRequest
	responses []outbound.ModelResponse
	err       error
}

func (c *recordingModelClient) Generate(_ context.Context, request outbound.ModelRequest) (outbound.ModelResponse, error) {
	c.requests = append(c.requests, request)
	if c.err != nil {
		return outbound.ModelResponse{}, c.err
	}
	if len(c.requests) > len(c.responses) {
		return outbound.ModelResponse{}, errors.New("missing model response")
	}

	return c.responses[len(c.requests)-1], nil
}

type fakeToolRunner struct {
	listErr error
	calls   []outbound.ToolCall
}

func newFakeToolRunner() *fakeToolRunner {
	return &fakeToolRunner{}
}

func (r *fakeToolRunner) ListTools(context.Context, outbound.ToolListRequest) ([]outbound.ToolDefinition, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}

	return []outbound.ToolDefinition{
		{Name: "echo", Description: "Returns text"},
	}, nil
}

func (r *fakeToolRunner) RunTool(_ context.Context, call outbound.ToolCall) (outbound.ToolResult, error) {
	r.calls = append(r.calls, call)
	if call.Name != "echo" {
		return outbound.ToolResult{Content: "unknown tool: " + call.Name, IsError: true}, nil
	}

	return outbound.ToolResult{Content: call.Arguments["text"]}, nil
}

func assertMessage(t *testing.T, message outbound.ModelMessage, role string, contentContains string) {
	t.Helper()

	if message.Role != role {
		t.Fatalf("Role = %q, want %q", message.Role, role)
	}
	if !strings.Contains(message.Content, contentContains) {
		t.Fatalf("Content = %q, want to contain %q", message.Content, contentContains)
	}
}
