package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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
	assertTurn(t, result.AgentTurns, 0, wantTurn{
		index:           1,
		status:          run.AgentTurnStatusNaturalLanguageFinal,
		message:         "model returned natural-language final answer",
		responsePreview: "done",
	})
	if model.requests[0].RunID != "run_test" {
		t.Fatalf("model RunID = %s, want run_test", model.requests[0].RunID)
	}
	assertMessage(t, model.requests[0].Messages[0], "system", "Available tools")
	assertMessage(t, model.requests[0].Messages[0], "system", "workspace: Lists or stats workspace paths without reading file contents.")
	assertMessage(t, model.requests[0].Messages[0], "system", "sandbox_exec: Runs a command inside the sandbox from /workspace/work.")
	assertMessage(t, model.requests[0].Messages[1], "user", "do work")
	if !strings.Contains(result.SystemPrompt, "Available tools") {
		t.Fatalf("SystemPrompt = %q, want available tools", result.SystemPrompt)
	}
}

func TestNewRunnerUsesEightDefaultMaxTurns(t *testing.T) {
	runner := NewRunner(nil, newFakeToolRunner())
	if runner.maxTurns != 8 {
		t.Fatalf("maxTurns = %d, want 8", runner.maxTurns)
	}
}

func TestRunnerExposesSandboxVerificationPolicyToModel(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{{Content: `{"type":"final","summary":"done"}`}},
	}
	tools := newFakeToolRunnerWithTools([]outbound.ToolDefinition{
		{Name: "workspace", Description: "Lists or stats workspace paths without reading file contents."},
		{Name: "sandbox_exec", Description: "Runs a command inside the sandbox from /workspace/work."},
	})
	runner := NewRunner(model, tools)

	result, err := runner.Run(context.Background(), RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "count the words exactly"},
		Context: outbound.ToolContext{
			Workspace: testToolWorkspace(),
			Sandbox: outbound.Sandbox{
				ID:               "sandbox_test",
				SupportsCommands: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("run agent: %v", err)
	}
	systemMessage := model.requests[0].Messages[0]
	assertMessage(t, systemMessage, "system", "call sandbox_exec before final")
	assertMessage(t, systemMessage, "system", "Do not guess exact answers when sandbox_exec can verify them.")
	assertMessage(t, systemMessage, "system", "arithmetic, counts, searches, file content inspection")
	if !strings.Contains(result.SystemPrompt, "exact or verifiable answer") {
		t.Fatalf("SystemPrompt = %q, want sandbox verification policy", result.SystemPrompt)
	}
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
	assertTurn(t, result.AgentTurns, 0, wantTurn{
		index:           1,
		status:          run.AgentTurnStatusFinal,
		actionType:      run.AgentTurnActionTypeFinal,
		message:         "model returned final answer",
		responsePreview: "done",
	})
}

func TestRunnerIncludesUploadedFileMetadataInInitialMessage(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{{Content: `{"type":"final","summary":"done"}`}},
	}
	runner := NewRunner(model, newFakeToolRunner())

	result, err := runner.Run(context.Background(), RunRequest{
		RunID: "run_test",
		Task: run.TaskSpec{
			Prompt: "count this file",
			Attachments: []run.TaskAttachment{
				{
					Filename:  "README.md",
					MediaType: "text/markdown",
					Content:   []byte("# Demo\n"),
					SizeBytes: 7,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("run agent: %v", err)
	}
	if result.Summary != "done" {
		t.Fatalf("summary = %q, want done", result.Summary)
	}
	initialUserMessage := model.requests[0].Messages[1]
	assertMessage(t, initialUserMessage, "user", "count this file")
	assertMessage(t, initialUserMessage, "user", "Uploaded files available in the workspace:")
	assertMessage(t, initialUserMessage, "user", "path: README.md; media_type: text/markdown; size_bytes: 7")
	assertMessage(t, initialUserMessage, "user", "If the user refers to this file without naming it")
	if strings.Contains(initialUserMessage.Content, "# Demo") {
		t.Fatalf("initial user message exposed attachment content: %q", initialUserMessage.Content)
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
		timeUnix(103),
		timeUnix(104),
		timeUnix(105),
		timeUnix(106),
	)))

	result, err := runner.Run(context.Background(), RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "do work"},
		Context: outbound.ToolContext{
			Workspace: testToolWorkspace(),
			Sandbox:   outbound.Sandbox{ID: "sandbox_test"},
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
	assertTurn(t, result.AgentTurns, 0, wantTurn{
		index:      1,
		status:     run.AgentTurnStatusToolCall,
		actionType: run.AgentTurnActionTypeToolCall,
		toolName:   "echo",
		message:    "model requested tool call",
		startedAt:  timeUnix(101),
		endedAt:    timeUnix(102),
	})
	assertTurn(t, result.AgentTurns, 1, wantTurn{
		index:      2,
		status:     run.AgentTurnStatusFinal,
		actionType: run.AgentTurnActionTypeFinal,
		message:    "model returned final answer",
		startedAt:  timeUnix(105),
		endedAt:    timeUnix(106),
	})
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

func TestRunnerRejectsUnavailableToolBeforeToolRunner(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{
			{Content: `{"type":"tool_call","tool":"sh_script","arguments":{"script":"echo hi"}}`},
			{Content: `{"type":"final","summary":"answered without tool"}`},
		},
	}
	tools := newFakeToolRunnerWithTools(nil)
	runner := NewRunner(model, tools)

	result, err := runner.Run(context.Background(), RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "do work"},
	})
	if err != nil {
		t.Fatalf("run agent: %v", err)
	}
	if result.Summary != "answered without tool" {
		t.Fatalf("summary = %q, want answered without tool", result.Summary)
	}
	if result.ToolCallCount != 0 {
		t.Fatalf("ToolCallCount = %d, want 0", result.ToolCallCount)
	}
	if len(result.ToolCalls) != 0 {
		t.Fatalf("len(ToolCalls) = %d, want 0", len(result.ToolCalls))
	}
	if len(tools.calls) != 0 {
		t.Fatalf("len(tool calls) = %d, want 0", len(tools.calls))
	}
	assertTurn(t, result.AgentTurns, 0, wantTurn{
		index:           1,
		status:          run.AgentTurnStatusInvalidToolCall,
		actionType:      run.AgentTurnActionTypeToolCall,
		toolName:        "sh_script",
		message:         "tool is not available: sh_script",
		responsePreview: `{"type":"tool_call","tool":"sh_script","arguments":{"script":"echo hi"}}`,
	})
	assertTurn(t, result.AgentTurns, 1, wantTurn{
		index:      2,
		status:     run.AgentTurnStatusFinal,
		actionType: run.AgentTurnActionTypeFinal,
	})
	lastMessages := model.requests[1].Messages
	assertMessage(t, lastMessages[len(lastMessages)-1], "user", `The tool "sh_script" is not available.`)
	assertMessage(t, lastMessages[len(lastMessages)-1], "user", "Available tools: none")
	assertMessage(t, lastMessages[len(lastMessages)-1], "user", "Do not invent tool names.")
	if !strings.Contains(result.SystemPrompt, "Available tools:\n- none") {
		t.Fatalf("SystemPrompt = %q, want no available tools", result.SystemPrompt)
	}
}

func TestRunnerRecordsExistingToolResultError(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{
			{Content: `{"type":"tool_call","tool":"workspace","arguments":{"operation":"stat","area":"input","path":"missing.md"}}`},
			{Content: `{"type":"final","summary":"handled tool error"}`},
		},
	}
	tools := newFakeToolRunnerWithTools([]outbound.ToolDefinition{
		{Name: "workspace", Description: "Lists or stats workspace paths without reading file contents."},
	})
	tools.results = map[string]outbound.ToolResult{
		"workspace": {Content: "path is not available", IsError: true},
	}
	runner := NewRunner(model, tools)

	result, err := runner.Run(context.Background(), RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "do work"},
	})
	if err != nil {
		t.Fatalf("run agent: %v", err)
	}
	if result.ToolCallCount != 1 {
		t.Fatalf("ToolCallCount = %d, want 1", result.ToolCallCount)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(result.ToolCalls))
	}
	if result.ToolCalls[0].Name != "workspace" {
		t.Fatalf("tool record name = %q, want workspace", result.ToolCalls[0].Name)
	}
	if result.ToolCalls[0].Result != "path is not available" {
		t.Fatalf("tool record result = %q, want path is not available", result.ToolCalls[0].Result)
	}
	if !result.ToolCalls[0].IsError {
		t.Fatal("tool record IsError = false, want true")
	}
	if len(tools.calls) != 1 {
		t.Fatalf("len(tool calls) = %d, want 1", len(tools.calls))
	}
	lastMessages := model.requests[1].Messages
	assertMessage(t, lastMessages[len(lastMessages)-1], "user", "Tool error for workspace:\npath is not available")
}

func TestRunnerRejectsPlaceholderToolArgumentsAndContinues(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{
			{Content: `{"type":"tool_call","tool":"sandbox_exec","arguments":{"command":"wc -m <file_path>"}}`},
			{Content: `{"type":"tool_call","tool":"sandbox_exec","arguments":{"command":"wc -m /workspace/input/README.md"}}`},
			{Content: `{"type":"final","summary":"README.md has 123 characters"}`},
		},
	}
	tools := newFakeToolRunnerWithTools([]outbound.ToolDefinition{
		{Name: "workspace", Description: "Lists or stats workspace paths without reading file contents."},
		{Name: "sandbox_exec", Description: "Runs a command inside the sandbox from /workspace/work."},
	})
	tools.results = map[string]outbound.ToolResult{
		"sandbox_exec": {Content: "exit_code: 0\nstdout:\n123 /workspace/input/README.md\n"},
	}
	runner := NewRunner(model, tools)

	result, err := runner.Run(context.Background(), RunRequest{
		RunID: "run_test",
		Task: run.TaskSpec{
			Prompt: "count this file through sh",
			Attachments: []run.TaskAttachment{
				{Filename: "README.md", MediaType: "text/markdown", SizeBytes: 123},
			},
		},
		Context: outbound.ToolContext{
			Workspace: testToolWorkspaceWithFiles(),
			Sandbox: outbound.Sandbox{
				ID:               "sandbox_test",
				SupportsCommands: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("run agent: %v", err)
	}
	if result.Summary != "README.md has 123 characters" {
		t.Fatalf("summary = %q, want corrected final", result.Summary)
	}
	if result.ToolCallCount != 1 {
		t.Fatalf("ToolCallCount = %d, want 1", result.ToolCallCount)
	}
	if len(tools.calls) != 1 {
		t.Fatalf("len(tool calls) = %d, want only corrected shell call", len(tools.calls))
	}
	if tools.calls[0].Arguments["command"] != "wc -m /workspace/input/README.md" {
		t.Fatalf("sandbox_exec command = %q, want corrected file path", tools.calls[0].Arguments["command"])
	}
	assertTurn(t, result.AgentTurns, 0, wantTurn{
		index:           1,
		status:          run.AgentTurnStatusInvalidToolCall,
		actionType:      run.AgentTurnActionTypeToolCall,
		toolName:        "sandbox_exec",
		message:         "tool call arguments contain placeholder values",
		responsePreview: `{"type":"tool_call","tool":"sandbox_exec","arguments":{"command":"wc -m <file_path>"}}`,
	})
	assertTurn(t, result.AgentTurns, 1, wantTurn{
		index:      2,
		status:     run.AgentTurnStatusToolCall,
		actionType: run.AgentTurnActionTypeToolCall,
		toolName:   "sandbox_exec",
	})
	correction := model.requests[1].Messages[len(model.requests[1].Messages)-1]
	assertMessage(t, correction, "user", "placeholder argument values: command=<file_path>")
	assertMessage(t, correction, "user", "Uploaded files: README.md")
	assertMessage(t, correction, "user", "Available tools: sandbox_exec, workspace")
}

func TestRunnerAllowsAdvertisedSandboxExecTool(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{
			{Content: `{"type":"tool_call","tool":"sandbox_exec","arguments":{"command":"pwd"}}`},
			{Content: `{"type":"final","summary":"sandbox command ran"}`},
		},
	}
	tools := newFakeToolRunnerWithTools([]outbound.ToolDefinition{
		{Name: "sandbox_exec", Description: "Runs a command inside the sandbox from /workspace/work."},
	})
	tools.results = map[string]outbound.ToolResult{
		"sandbox_exec": {Content: "exit_code: 0\nstdout:\n/workspace/work\n"},
	}
	runner := NewRunner(model, tools)

	result, err := runner.Run(context.Background(), RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "do work"},
		Context: outbound.ToolContext{
			Workspace: testToolWorkspace(),
			Sandbox: outbound.Sandbox{
				ID:               "sandbox_test",
				SupportsCommands: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("run agent: %v", err)
	}
	if len(tools.calls) != 1 {
		t.Fatalf("len(tool calls) = %d, want 1", len(tools.calls))
	}
	if tools.calls[0].Name != "sandbox_exec" {
		t.Fatalf("tool name = %q, want sandbox_exec", tools.calls[0].Name)
	}
	if len(result.ToolCalls) != 1 || result.ToolCalls[0].Name != "sandbox_exec" {
		t.Fatalf("ToolCalls = %#v, want sandbox_exec record", result.ToolCalls)
	}
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
	assertMessage(t, lastMessages[len(lastMessages)-1], "user", "action type must be final or tool_call")
	assertMessage(t, lastMessages[len(lastMessages)-1], "user", "Do not return tool_result.")
	if len(tools.calls) != 0 {
		t.Fatalf("len(tool calls) = %d, want 0", len(tools.calls))
	}
	assertTurn(t, result.AgentTurns, 0, wantTurn{
		index:           1,
		status:          run.AgentTurnStatusProtocolError,
		message:         "action type must be final or tool_call",
		responsePreview: `{"type":"tool_result","result":"hello from tool"}`,
	})
	assertTurn(t, result.AgentTurns, 1, wantTurn{
		index:      2,
		status:     run.AgentTurnStatusFinal,
		actionType: run.AgentTurnActionTypeFinal,
	})
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

func TestRunnerParsesFencedJSONToolCallAndContinues(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{
			{Content: "```json\n{\"type\":\"tool_call\",\"tool\":\"echo\",\"arguments\":{\"text\":\"hello\"}}\n```"},
			{Content: `{"type":"final","summary":"fenced tool handled"}`},
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
	if result.Summary != "fenced tool handled" {
		t.Fatalf("summary = %q, want fenced tool handled", result.Summary)
	}
	if len(tools.calls) != 1 {
		t.Fatalf("len(tool calls) = %d, want 1", len(tools.calls))
	}
	if len(model.requests) != 2 {
		t.Fatalf("len(model requests) = %d, want 2", len(model.requests))
	}
	lastMessages := model.requests[1].Messages
	assertMessage(t, lastMessages[len(lastMessages)-2], "assistant", "```json")
	assertMessage(t, lastMessages[len(lastMessages)-1], "user", "Tool result for echo:\nhello")
	assertTurn(t, result.AgentTurns, 0, wantTurn{
		index:      1,
		status:     run.AgentTurnStatusToolCall,
		actionType: run.AgentTurnActionTypeToolCall,
		toolName:   "echo",
	})
	assertTurn(t, result.AgentTurns, 1, wantTurn{
		index:      2,
		status:     run.AgentTurnStatusFinal,
		actionType: run.AgentTurnActionTypeFinal,
	})
}

func TestRunnerParsesBooleanFinalSummary(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{{Content: `{"type":"final","summary":true}`}},
	}
	runner := NewRunner(model, newFakeToolRunner())

	result, err := runner.Run(context.Background(), RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "is 0.11 < 0.2?"},
	})
	if err != nil {
		t.Fatalf("run agent: %v", err)
	}
	if result.Summary != "true" {
		t.Fatalf("summary = %q, want true", result.Summary)
	}
	assertTurn(t, result.AgentTurns, 0, wantTurn{
		index:      1,
		status:     run.AgentTurnStatusFinal,
		actionType: run.AgentTurnActionTypeFinal,
	})
}

func TestRunnerParsesNumericToolArgument(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{
			{Content: `{"type":"tool_call","tool":"echo","arguments":{"text":30}}`},
			{Content: `{"type":"final","summary":"done"}`},
		},
	}
	tools := newFakeToolRunner()
	runner := NewRunner(model, tools)

	_, err := runner.Run(context.Background(), RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "do work"},
	})
	if err != nil {
		t.Fatalf("run agent: %v", err)
	}
	if tools.calls[0].Arguments["text"] != "30" {
		t.Fatalf("tool text argument = %q, want 30", tools.calls[0].Arguments["text"])
	}
}

func TestRunnerProvidesTargetedCorrectionForInvalidSummary(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{
			{Content: `{"type":"final","summary":{"value":"done"}}`},
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
	lastMessages := model.requests[1].Messages
	assertMessage(t, lastMessages[len(lastMessages)-1], "user", "final.summary must be a string, boolean, or number")
	assertTurn(t, result.AgentTurns, 0, wantTurn{
		index:   1,
		status:  run.AgentTurnStatusProtocolError,
		message: "final.summary must be a string, boolean, or number",
	})
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
		timeUnix(103),
		timeUnix(104),
		timeUnix(105),
		timeUnix(106),
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
	assertTurn(t, result.AgentTurns, 0, wantTurn{
		index:      1,
		status:     run.AgentTurnStatusToolCall,
		actionType: run.AgentTurnActionTypeToolCall,
		toolName:   "echo",
		startedAt:  timeUnix(101),
		endedAt:    timeUnix(102),
	})
	assertTurn(t, result.AgentTurns, 1, wantTurn{
		index:     2,
		status:    run.AgentTurnStatusMaxTurns,
		message:   "agent reached max turns",
		startedAt: timeUnix(105),
		endedAt:   timeUnix(106),
	})
}

func TestRunnerReturnsErrorWhenProtocolErrorsExceedMaxTurns(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{{Content: `{"type":"tool_result","result":"hello from tool"}`}},
	}
	tools := newFakeToolRunner()
	runner := NewRunner(model, tools, WithMaxTurns(1))

	result, err := runner.Run(context.Background(), RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "do work"},
	})
	assertAgentErrorCode(t, err, ErrorCodeAgentMaxTurns)
	if len(tools.calls) != 0 {
		t.Fatalf("len(tool calls) = %d, want 0", len(tools.calls))
	}
	assertTurn(t, result.AgentTurns, 0, wantTurn{
		index:           1,
		status:          run.AgentTurnStatusProtocolError,
		responsePreview: `{"type":"tool_result","result":"hello from tool"}`,
	})
	assertTurn(t, result.AgentTurns, 1, wantTurn{
		index:  2,
		status: run.AgentTurnStatusMaxTurns,
	})
}

func TestRunnerReturnsMaxTurnsForRepeatedUnavailableToolCalls(t *testing.T) {
	model := &repeatingModelClient{
		response: outbound.ModelResponse{
			Content: `{"type":"tool_call","tool":"sh_script","arguments":{"script":"echo hi"}}`,
		},
	}
	tools := newFakeToolRunnerWithTools(nil)
	runner := NewRunner(model, tools, WithMaxTurns(2))

	result, err := runner.Run(context.Background(), RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "do work"},
	})
	assertAgentErrorCode(t, err, ErrorCodeAgentMaxTurns)
	if len(tools.calls) != 0 {
		t.Fatalf("len(tool calls) = %d, want 0", len(tools.calls))
	}
	if len(result.ToolCalls) != 0 {
		t.Fatalf("len(ToolCalls) = %d, want 0", len(result.ToolCalls))
	}
	assertTurn(t, result.AgentTurns, 0, wantTurn{
		index:      1,
		status:     run.AgentTurnStatusInvalidToolCall,
		actionType: run.AgentTurnActionTypeToolCall,
		toolName:   "sh_script",
	})
	assertTurn(t, result.AgentTurns, 1, wantTurn{
		index:      2,
		status:     run.AgentTurnStatusInvalidToolCall,
		actionType: run.AgentTurnActionTypeToolCall,
		toolName:   "sh_script",
	})
	assertTurn(t, result.AgentTurns, 2, wantTurn{
		index:  3,
		status: run.AgentTurnStatusMaxTurns,
	})
	if !strings.Contains(result.SystemPrompt, "Available tools:\n- none") {
		t.Fatalf("SystemPrompt = %q, want no available tools", result.SystemPrompt)
	}
}

func TestRunnerPropagatesModelErrors(t *testing.T) {
	model := &recordingModelClient{err: errModelFailed}
	runner := NewRunner(model, newFakeToolRunner())

	result, err := runner.Run(context.Background(), RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "do work"},
	})
	if !errors.Is(err, errModelFailed) {
		t.Fatalf("Run() error = %v, want %v", err, errModelFailed)
	}
	assertAgentErrorCode(t, err, ErrorCodeModelGenerateFailed)
	assertTurn(t, result.AgentTurns, 0, wantTurn{
		index:   1,
		status:  run.AgentTurnStatusModelError,
		message: "model generation failed",
	})
}

func TestRunnerReturnsPartialToolCallsWhenModelFailsAfterToolCall(t *testing.T) {
	model := &failingAfterToolModelClient{}
	tools := newFakeToolRunner()
	runner := NewRunner(model, tools, WithClock(sequenceClock(
		timeUnix(101),
		timeUnix(102),
		timeUnix(103),
		timeUnix(104),
		timeUnix(105),
		timeUnix(106),
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
	assertTurn(t, result.AgentTurns, 0, wantTurn{
		index:      1,
		status:     run.AgentTurnStatusToolCall,
		actionType: run.AgentTurnActionTypeToolCall,
		toolName:   "echo",
	})
	assertTurn(t, result.AgentTurns, 1, wantTurn{
		index:   2,
		status:  run.AgentTurnStatusModelError,
		message: "model generation failed",
	})
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
		timeUnix(103),
		timeUnix(104),
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
	if !record.StartedAt.Equal(timeUnix(103)) || !record.EndedAt.Equal(timeUnix(104)) {
		t.Fatalf("record timestamps = %v/%v, want deterministic clock", record.StartedAt, record.EndedAt)
	}
	assertTurn(t, result.AgentTurns, 0, wantTurn{
		index:      1,
		status:     run.AgentTurnStatusToolCall,
		actionType: run.AgentTurnActionTypeToolCall,
		toolName:   "echo",
		startedAt:  timeUnix(101),
		endedAt:    timeUnix(102),
	})
}

func TestRunnerReturnsFinalSummaryInvalidForEmptyNaturalLanguageOutput(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{{Content: "   "}},
	}
	runner := NewRunner(model, newFakeToolRunner())

	result, err := runner.Run(context.Background(), RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "do work"},
	})

	assertAgentErrorCode(t, err, ErrorCodeFinalSummaryInvalid)
	assertTurn(t, result.AgentTurns, 0, wantTurn{
		index:      1,
		status:     run.AgentTurnStatusFinal,
		actionType: run.AgentTurnActionTypeFinal,
		message:    "agent final summary is invalid",
	})
}

func TestRunnerBoundsTurnResponsePreviewWithoutBreakingUTF8(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{
			{Content: `{"type":"final","summary":"` + strings.Repeat("界", run.MaxAgentTurnPreviewLength) + `"}`},
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

	preview := result.AgentTurns[0].ResponsePreview
	if len(preview) > run.MaxAgentTurnPreviewLength {
		t.Fatalf("len(ResponsePreview) = %d, want <= %d", len(preview), run.MaxAgentTurnPreviewLength)
	}
	if !utf8.ValidString(preview) {
		t.Fatalf("ResponsePreview is not valid UTF-8: %q", preview)
	}
	if !strings.HasSuffix(preview, agentTurnPreviewTruncatedMarker) {
		t.Fatalf("ResponsePreview does not contain truncation marker: %q", preview)
	}
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

type repeatingModelClient struct {
	response outbound.ModelResponse
	requests []outbound.ModelRequest
}

func (c *repeatingModelClient) Generate(
	_ context.Context,
	request outbound.ModelRequest,
) (outbound.ModelResponse, error) {
	c.requests = append(c.requests, request)

	return c.response, nil
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
	tools        []outbound.ToolDefinition
	toolsSet     bool
	results      map[string]outbound.ToolResult
	listRequests []outbound.ToolListRequest
	calls        []outbound.ToolCall
}

func newFakeToolRunner() *fakeToolRunner {
	return &fakeToolRunner{}
}

func newFakeToolRunnerWithTools(tools []outbound.ToolDefinition) *fakeToolRunner {
	return &fakeToolRunner{tools: tools, toolsSet: true}
}

func (r *fakeToolRunner) ListTools(_ context.Context, request outbound.ToolListRequest) ([]outbound.ToolDefinition, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	r.listRequests = append(r.listRequests, request)
	if r.toolsSet {
		return r.tools, nil
	}

	return []outbound.ToolDefinition{
		{Name: "echo", Description: "Returns text"},
		{Name: "workspace", Description: "Lists or stats workspace paths without reading file contents."},
		{Name: "sandbox_exec", Description: "Runs a command inside the sandbox from /workspace/work."},
	}, nil
}

func (r *fakeToolRunner) RunTool(_ context.Context, call outbound.ToolCall) (outbound.ToolResult, error) {
	r.calls = append(r.calls, call)
	if r.runErr != nil {
		return outbound.ToolResult{}, r.runErr
	}
	if result, ok := r.results[call.Name]; ok {
		return result, nil
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
	if !record.StartedAt.Equal(timeUnix(103)) {
		t.Fatalf("StartedAt = %v, want %v", record.StartedAt, timeUnix(103))
	}
	if !record.EndedAt.Equal(timeUnix(104)) {
		t.Fatalf("EndedAt = %v, want %v", record.EndedAt, timeUnix(104))
	}
}

type wantTurn struct {
	index           int
	status          string
	actionType      string
	toolName        string
	message         string
	responsePreview string
	startedAt       time.Time
	endedAt         time.Time
}

func assertTurn(t *testing.T, turns []TurnRecord, index int, want wantTurn) {
	t.Helper()

	if len(turns) <= index {
		t.Fatalf("len(AgentTurns) = %d, want at least %d", len(turns), index+1)
	}
	turn := turns[index]
	if turn.Index != want.index {
		t.Fatalf("AgentTurns[%d].Index = %d, want %d", index, turn.Index, want.index)
	}
	if turn.Status != want.status {
		t.Fatalf("AgentTurns[%d].Status = %q, want %q", index, turn.Status, want.status)
	}
	if want.actionType != "" && turn.ActionType != want.actionType {
		t.Fatalf("AgentTurns[%d].ActionType = %q, want %q", index, turn.ActionType, want.actionType)
	}
	if want.toolName != "" && turn.ToolName != want.toolName {
		t.Fatalf("AgentTurns[%d].ToolName = %q, want %q", index, turn.ToolName, want.toolName)
	}
	if want.message != "" && turn.Message != want.message {
		t.Fatalf("AgentTurns[%d].Message = %q, want %q", index, turn.Message, want.message)
	}
	if want.responsePreview != "" && turn.ResponsePreview != want.responsePreview {
		t.Fatalf("AgentTurns[%d].ResponsePreview = %q, want %q", index, turn.ResponsePreview, want.responsePreview)
	}
	if !want.startedAt.IsZero() && !turn.StartedAt.Equal(want.startedAt) {
		t.Fatalf("AgentTurns[%d].StartedAt = %v, want %v", index, turn.StartedAt, want.startedAt)
	}
	if !want.endedAt.IsZero() && !turn.EndedAt.Equal(want.endedAt) {
		t.Fatalf("AgentTurns[%d].EndedAt = %v, want %v", index, turn.EndedAt, want.endedAt)
	}
}

func assertEchoToolInvocation(t *testing.T, tools *fakeToolRunner) {
	t.Helper()

	if len(tools.listRequests) != 1 {
		t.Fatalf("len(list requests) = %d, want 1", len(tools.listRequests))
	}
	if tools.listRequests[0].Context.Workspace != testToolWorkspace() {
		t.Fatalf("list tools workspace = %#v, want test workspace", tools.listRequests[0].Context.Workspace)
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
	if tools.calls[0].Context.Workspace != testToolWorkspace() {
		t.Fatalf("tool workspace = %#v, want test workspace", tools.calls[0].Context.Workspace)
	}
}

func testToolWorkspace() outbound.Workspace {
	return outbound.Workspace{
		RootPath:  "/tmp/workspace",
		InputPath: "/tmp/workspace/input",
		WorkPath:  "/tmp/workspace/work",
	}
}

func testToolWorkspaceWithFiles() outbound.Workspace {
	workspace := testToolWorkspace()
	workspace.HasFiles = true

	return workspace
}
