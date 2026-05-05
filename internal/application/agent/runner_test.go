package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestRunnerTreatsNaturalLanguageResponseAsFinalSummary(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{{Content: "done"}},
	}
	tools := newFakeToolRunner()
	runner := NewRunner(model, tools)

	result, err := runner.Run(context.Background(), RunRequest{
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
	if len(result.ToolCalls) != 0 {
		t.Fatalf("len(ToolCalls) = %d, want 0", len(result.ToolCalls))
	}
	if model.requests[0].RunID != "run_test" {
		t.Fatalf("model RunID = %s, want run_test", model.requests[0].RunID)
	}
	assertMessage(t, model.requests[0].Messages[0], "system", "Available tools")
	assertMessage(t, model.requests[0].Messages[0], "system", "list_files: Lists files")
	assertMessage(t, model.requests[0].Messages[0], "system", "read_file: Reads text files")
	assertMessage(t, model.requests[0].Messages[1], "user", "do work")
}

func TestRunnerReturnsJSONFinalActionSummary(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{{Content: `{"type":"final","summary":"done"}`}},
	}
	runner := NewRunner(model, newFakeToolRunner())

	result, err := runner.Run(context.Background(), RunRequest{
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
	runner := NewRunner(model, tools, WithClock(sequenceClock(
		timeUnix(101),
		timeUnix(102),
	)))

	result, err := runner.Run(context.Background(), RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "do work"},
		Context: outbound.ToolContext{
			WorkspacePath: "/tmp/workspace",
			Sandbox:       outbound.Sandbox{ID: "sandbox_test"},
		},
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
	assertEchoToolRecord(t, result.ToolCalls)
	assertEchoToolInvocation(t, tools)
	if len(model.requests) != 2 {
		t.Fatalf("len(model requests) = %d, want 2", len(model.requests))
	}
	lastMessages := model.requests[1].Messages
	assertMessage(t, lastMessages[len(lastMessages)-2], "assistant", `"type":"tool_call"`)
	assertMessage(t, lastMessages[len(lastMessages)-1], "user", "Tool result for echo:\nhello")
	tools.calls[0].Arguments["text"] = "changed"
	if result.ToolCalls[0].Arguments["text"] != "hello" {
		t.Fatalf("record text after mutation = %q, want hello", result.ToolCalls[0].Arguments["text"])
	}
}

func TestRunnerFeedsUnknownToolResultBackToModel(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{
			{Content: `{"type":"tool_call","tool":"missing","arguments":{}}`},
			{Content: `{"type":"final","summary":"handled tool error"}`},
		},
	}
	tools := newFakeToolRunner()
	runner := NewRunner(model, tools)

	result, err := runner.Run(context.Background(), RunRequest{
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
	if len(result.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(result.ToolCalls))
	}
	if result.ToolCalls[0].Name != "missing" {
		t.Fatalf("tool record name = %q, want missing", result.ToolCalls[0].Name)
	}
	if result.ToolCalls[0].Result != "unknown tool: missing" {
		t.Fatalf("tool record result = %q, want unknown tool", result.ToolCalls[0].Result)
	}
	if !result.ToolCalls[0].IsError {
		t.Fatal("tool record IsError = false, want true")
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
	runner := NewRunner(model, tools)

	result, err := runner.Run(context.Background(), RunRequest{
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
	runner := NewRunner(model, tools)

	result, err := runner.Run(context.Background(), RunRequest{
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
	runner := NewRunner(model, tools)

	result, err := runner.Run(context.Background(), RunRequest{
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
	runner := NewRunner(model, newFakeToolRunner())

	result, err := runner.Run(context.Background(), RunRequest{
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
	runner := NewRunner(model, tools)

	result, err := runner.Run(context.Background(), RunRequest{
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
	runner := NewRunner(model, tools, WithMaxTurns(1), WithClock(sequenceClock(
		timeUnix(101),
		timeUnix(102),
	)))

	result, err := runner.Run(context.Background(), RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "do work"},
	})
	assertAgentErrorCode(t, err, ErrorCodeAgentMaxTurns)
	if len(tools.calls) != 1 {
		t.Fatalf("len(tool calls) = %d, want 1", len(tools.calls))
	}
	assertEchoToolRecord(t, result.ToolCalls)
}

func TestRunnerReturnsErrorWhenProtocolErrorsExceedMaxTurns(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{{Content: `{"type":"tool_result","result":"hello from tool"}`}},
	}
	tools := newFakeToolRunner()
	runner := NewRunner(model, tools, WithMaxTurns(1))

	_, err := runner.Run(context.Background(), RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "do work"},
	})
	assertAgentErrorCode(t, err, ErrorCodeAgentMaxTurns)
	if len(tools.calls) != 0 {
		t.Fatalf("len(tool calls) = %d, want 0", len(tools.calls))
	}
}

func TestRunnerPropagatesModelErrors(t *testing.T) {
	model := &recordingModelClient{err: errModelFailed}
	runner := NewRunner(model, newFakeToolRunner())

	_, err := runner.Run(context.Background(), RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "do work"},
	})
	if !errors.Is(err, errModelFailed) {
		t.Fatalf("Run() error = %v, want %v", err, errModelFailed)
	}
	assertAgentErrorCode(t, err, ErrorCodeModelGenerateFailed)
}

func TestRunnerReturnsPartialToolCallsWhenModelFailsAfterToolCall(t *testing.T) {
	model := &failingAfterToolModelClient{}
	tools := newFakeToolRunner()
	runner := NewRunner(model, tools, WithClock(sequenceClock(
		timeUnix(101),
		timeUnix(102),
	)))

	result, err := runner.Run(context.Background(), RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "do work"},
	})
	if !errors.Is(err, errModelFailed) {
		t.Fatalf("Run() error = %v, want %v", err, errModelFailed)
	}
	assertAgentErrorCode(t, err, ErrorCodeModelGenerateFailed)
	if result.ToolCallCount != 1 {
		t.Fatalf("ToolCallCount = %d, want 1", result.ToolCallCount)
	}
	assertEchoToolRecord(t, result.ToolCalls)
}

func TestRunnerPropagatesListToolsErrors(t *testing.T) {
	model := &recordingModelClient{responses: []outbound.ModelResponse{{Content: "done"}}}
	tools := &fakeToolRunner{listErr: errListToolsFailed}
	runner := NewRunner(model, tools)

	_, err := runner.Run(context.Background(), RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "do work"},
	})
	if !errors.Is(err, errListToolsFailed) {
		t.Fatalf("Run() error = %v, want %v", err, errListToolsFailed)
	}
	assertAgentErrorCode(t, err, ErrorCodeToolListFailed)
	if len(model.requests) != 0 {
		t.Fatalf("len(model requests) = %d, want 0", len(model.requests))
	}
}

func TestRunnerRecordsToolExecutionErrorAndReturnsPartialResult(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{
			{Content: `{"type":"tool_call","tool":"echo","arguments":{"text":"hello"}}`},
		},
	}
	tools := &fakeToolRunner{runErr: errToolRunFailed}
	runner := NewRunner(model, tools, WithClock(sequenceClock(
		timeUnix(101),
		timeUnix(102),
	)))

	result, err := runner.Run(context.Background(), RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "do work"},
	})
	if !errors.Is(err, errToolRunFailed) {
		t.Fatalf("Run() error = %v, want %v", err, errToolRunFailed)
	}
	assertAgentErrorCode(t, err, ErrorCodeToolExecutionFailed)
	if result.ToolCallCount != 0 {
		t.Fatalf("ToolCallCount = %d, want 0", result.ToolCallCount)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(result.ToolCalls))
	}
	record := result.ToolCalls[0]
	if record.Result != "tool execution failed" {
		t.Fatalf("record result = %q, want tool execution failed", record.Result)
	}
	if !record.IsError {
		t.Fatal("record IsError = false, want true")
	}
	if !record.StartedAt.Equal(timeUnix(101)) || !record.EndedAt.Equal(timeUnix(102)) {
		t.Fatalf("record timestamps = %v/%v, want deterministic clock", record.StartedAt, record.EndedAt)
	}
}

func TestRunnerReturnsFinalSummaryInvalidForEmptyNaturalLanguageOutput(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{{Content: "   "}},
	}
	runner := NewRunner(model, newFakeToolRunner())

	_, err := runner.Run(context.Background(), RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "do work"},
	})

	assertAgentErrorCode(t, err, ErrorCodeFinalSummaryInvalid)
}

var errModelFailed = errors.New("model failed")
var errListToolsFailed = errors.New("list tools failed")
var errToolRunFailed = errors.New("tool failed")

func timeUnix(seconds int64) time.Time {
	return time.Unix(seconds, 0).UTC()
}

func sequenceClock(times ...time.Time) func() time.Time {
	index := 0

	return func() time.Time {
		if index >= len(times) {
			return times[len(times)-1]
		}

		value := times[index]
		index++

		return value
	}
}

type recordingModelClient struct {
	requests  []outbound.ModelRequest
	responses []outbound.ModelResponse
	err       error
}

type failingAfterToolModelClient struct {
	calls int
}

func (c *failingAfterToolModelClient) Generate(
	context.Context,
	outbound.ModelRequest,
) (outbound.ModelResponse, error) {
	c.calls++
	if c.calls == 1 {
		return outbound.ModelResponse{
			Content: `{"type":"tool_call","tool":"echo","arguments":{"text":"hello"}}`,
		}, nil
	}

	return outbound.ModelResponse{}, errModelFailed
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
	listErr      error
	runErr       error
	listRequests []outbound.ToolListRequest
	calls        []outbound.ToolCall
}

func newFakeToolRunner() *fakeToolRunner {
	return &fakeToolRunner{}
}

func (r *fakeToolRunner) ListTools(_ context.Context, request outbound.ToolListRequest) ([]outbound.ToolDefinition, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	r.listRequests = append(r.listRequests, request)

	return []outbound.ToolDefinition{
		{Name: "echo", Description: "Returns text"},
		{Name: "list_files", Description: "Lists files"},
		{Name: "read_file", Description: "Reads text files"},
	}, nil
}

func (r *fakeToolRunner) RunTool(_ context.Context, call outbound.ToolCall) (outbound.ToolResult, error) {
	r.calls = append(r.calls, call)
	if r.runErr != nil {
		return outbound.ToolResult{}, r.runErr
	}
	if call.Name != "echo" {
		return outbound.ToolResult{Content: "unknown tool: " + call.Name, IsError: true}, nil
	}

	return outbound.ToolResult{Content: call.Arguments["text"]}, nil
}

func assertAgentErrorCode(t *testing.T, err error, code ErrorCode) {
	t.Helper()

	var agentErr Error
	if !errors.As(err, &agentErr) {
		t.Fatalf("Run() error = %T %v, want agent Error", err, err)
	}
	if agentErr.Code != code {
		t.Fatalf("agent Error.Code = %q, want %q", agentErr.Code, code)
	}
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

func assertEchoToolRecord(t *testing.T, records []ToolCallRecord) {
	t.Helper()

	if len(records) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(records))
	}
	record := records[0]
	if record.Name != "echo" {
		t.Fatalf("record name = %q, want echo", record.Name)
	}
	if record.Arguments["text"] != "hello" {
		t.Fatalf("record text = %q, want hello", record.Arguments["text"])
	}
	if record.Result != "hello" {
		t.Fatalf("record result = %q, want hello", record.Result)
	}
	if record.IsError {
		t.Fatal("record IsError = true, want false")
	}
	if !record.StartedAt.Equal(timeUnix(101)) {
		t.Fatalf("StartedAt = %v, want %v", record.StartedAt, timeUnix(101))
	}
	if !record.EndedAt.Equal(timeUnix(102)) {
		t.Fatalf("EndedAt = %v, want %v", record.EndedAt, timeUnix(102))
	}
}

func assertEchoToolInvocation(t *testing.T, tools *fakeToolRunner) {
	t.Helper()

	if len(tools.listRequests) != 1 {
		t.Fatalf("len(list requests) = %d, want 1", len(tools.listRequests))
	}
	if tools.listRequests[0].Context.WorkspacePath != "/tmp/workspace" {
		t.Fatalf("list tools workspace path = %q, want /tmp/workspace", tools.listRequests[0].Context.WorkspacePath)
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
	if tools.calls[0].Context.Sandbox.ID != "sandbox_test" {
		t.Fatalf("tool sandbox = %q, want sandbox_test", tools.calls[0].Context.Sandbox.ID)
	}
	if tools.calls[0].Context.WorkspacePath != "/tmp/workspace" {
		t.Fatalf("tool workspace path = %q, want /tmp/workspace", tools.calls[0].Context.WorkspacePath)
	}
}
