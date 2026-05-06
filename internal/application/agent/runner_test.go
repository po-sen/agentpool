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

func TestRunnerRejectsNaturalLanguageResponseAndContinues(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{
			{Content: "done"},
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
	if result.ToolCallCount != 0 {
		t.Fatalf("ToolCallCount = %d, want 0", result.ToolCallCount)
	}
	if len(result.ToolCalls) != 0 {
		t.Fatalf("len(ToolCalls) = %d, want 0", len(result.ToolCalls))
	}
	assertTurn(t, result.AgentTurns, 0, wantTurn{
		index:               1,
		status:              run.AgentTurnStatusProtocolError,
		message:             "model response was not valid JSON",
		requestMessageCount: 2,
		requestContains:     "do work",
		rawResponse:         "done",
		responseFormat:      modelResponseFormatPlainText,
		protocolErrorCode:   actionParseCodeInvalidJSON,
		correctionContains:  "Return only one raw JSON object",
		responsePreview:     "done",
	})
	assertTurn(t, result.AgentTurns, 1, wantTurn{
		index:      2,
		status:     run.AgentTurnStatusFinal,
		actionType: run.AgentTurnActionTypeFinal,
		message:    "model returned final answer",
	})
	if model.requests[0].RunID != "run_test" {
		t.Fatalf("model RunID = %s, want run_test", model.requests[0].RunID)
	}
	assertMessage(t, requestMessages(model.requests[0])[0], "runtime", "Available tools")
	assertInstruction(t, model.requests[0], "Available tools")
	assertInstruction(t, model.requests[0], "workspace: Lists or stats workspace paths without reading file contents.")
	assertInstruction(t, model.requests[0], "sandbox_exec: Runs a command inside the sandbox from /workspace/work.")
	if len(model.requests[0].Tools) != 3 {
		t.Fatalf("len(model Tools) = %d, want advertised tools", len(model.requests[0].Tools))
	}
	assertMessage(t, requestMessages(model.requests[0])[1], "user", "do work")
	if len(model.requests) != 2 {
		t.Fatalf("len(model requests) = %d, want 2", len(model.requests))
	}
	lastMessages := requestMessages(model.requests[1])
	assertMessage(t, lastMessages[len(lastMessages)-1], "runtime", "Return exactly one JSON object")
}

func TestRunnerRecordsProviderRequestMessagesFromModelResponse(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{
			{
				Content: `{"type":"final","summary":"done"}`,
				RequestMessages: []outbound.ModelRequestMessage{
					{Role: "developer", Content: "system prompt"},
					{Role: "user", Content: "do work"},
				},
			},
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
	if len(result.AgentTurns) != 1 {
		t.Fatalf("len(AgentTurns) = %d, want 1", len(result.AgentTurns))
	}
	messages := result.AgentTurns[0].RequestMessages
	if len(messages) != 2 {
		t.Fatalf("len(RequestMessages) = %d, want 2", len(messages))
	}
	assertMessage(t, messages[0], "developer", "system prompt")
	if messages[0].Kind != "" {
		t.Fatalf("provider request message kind = %q, want empty", messages[0].Kind)
	}
	assertMessage(t, messages[1], "user", "do work")
}

func TestRunnerReportsProgressBeforeAndAfterModelTurn(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{{Content: `{"type":"final","summary":"done"}`}},
	}
	runner := NewRunner(model, newFakeToolRunner())
	progress := []RunProgress{}

	result, err := runner.Run(context.Background(), RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "do work"},
		ProgressObserver: func(_ context.Context, item RunProgress) error {
			progress = append(progress, item)

			return nil
		},
	})
	if err != nil {
		t.Fatalf("run agent: %v", err)
	}
	if result.Summary != "done" {
		t.Fatalf("summary = %q, want done", result.Summary)
	}
	if len(progress) != 2 {
		t.Fatalf("len(progress) = %d, want 2", len(progress))
	}
	assertTurn(t, progress[0].AgentTurns, 0, wantTurn{
		index:   1,
		status:  run.AgentTurnStatusModelResponse,
		message: "waiting for model response",
	})
	assertTurn(t, progress[1].AgentTurns, 0, wantTurn{
		index:      1,
		status:     run.AgentTurnStatusFinal,
		actionType: run.AgentTurnActionTypeFinal,
		message:    "model returned final answer",
	})
}

func TestNewRunnerUsesSixteenDefaultMaxTurns(t *testing.T) {
	runner := NewRunner(nil, newFakeToolRunner())
	if runner.maxTurns != 16 {
		t.Fatalf("maxTurns = %d, want 16", runner.maxTurns)
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

	_, err := runner.Run(context.Background(), RunRequest{
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
	assertMessage(t, requestMessages(model.requests[0])[0], "runtime", "exact/verifiable tasks")
	assertInstruction(t, model.requests[0], "call it before final")
	assertInstruction(t, model.requests[0], "do not guess answers it can verify.")
	assertInstruction(t, model.requests[0], "arithmetic, counts, searches, file inspection")
	assertInstruction(t, model.requests[0], "PDF/Office/images")
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
		rawResponse:     `{"type":"final","summary":"done"}`,
		responseFormat:  modelResponseFormatJSONObject,
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
	initialMessages := requestMessages(model.requests[0])
	assertMessage(t, initialMessages[1], "user", "count this file")
	workspaceContext := initialMessages[2]
	if workspaceContext.Kind != string(outbound.ModelPartKindWorkspaceContext) {
		t.Fatalf("workspace context kind = %q, want %q", workspaceContext.Kind, outbound.ModelPartKindWorkspaceContext)
	}
	assertMessage(t, workspaceContext, "user", "Workspace input files available to tools:")
	assertMessage(t, workspaceContext, "user", "path: README.md; virtual_path: /workspace/input/README.md; media_type: text/markdown; size_bytes: 7")
	assertMessage(t, workspaceContext, "user", "If the user refers to this file without naming it")
	if strings.Contains(workspaceContext.Content, "# Demo") {
		t.Fatalf("initial workspace context exposed attachment content: %q", workspaceContext.Content)
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
	lastMessages := requestMessages(model.requests[1])
	assertMessage(t, lastMessages[len(lastMessages)-2], "assistant", `"type":"tool_call"`)
	assertMessage(t, lastMessages[len(lastMessages)-1], "tool", "Tool result for echo:\nhello")
	tools.calls[0].Arguments["text"] = "changed"
	if result.ToolCalls[0].Arguments["text"] != "hello" {
		t.Fatalf("record text after mutation = %q, want hello", result.ToolCalls[0].Arguments["text"])
	}
}

func TestRunnerCallsProviderStyleToolCallTextAndReturnsFinalSummary(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{
			{Content: `{"name":"sandbox_exec","arguments":{"command":"printf '%s\n' \"$((123 * 321))\""}}`},
			{Content: `{"type":"final","summary":"123 * 321 = 39483"}`},
		},
	}
	tools := newFakeToolRunnerWithTools([]outbound.ToolDefinition{
		{Name: "sandbox_exec", Description: "Runs a command inside the sandbox from /workspace/work."},
	})
	tools.results = map[string]outbound.ToolResult{
		"sandbox_exec": {Content: "exit_code: 0\nstdout:\n39483\n"},
	}
	runner := NewRunner(model, tools)

	result, err := runner.Run(context.Background(), RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "告訴我 123 * 321 = ?"},
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
	if result.Summary != "123 * 321 = 39483" {
		t.Fatalf("summary = %q, want computed result", result.Summary)
	}
	if result.ToolCallCount != 1 {
		t.Fatalf("ToolCallCount = %d, want 1", result.ToolCallCount)
	}
	if len(tools.calls) != 1 {
		t.Fatalf("len(tool calls) = %d, want 1", len(tools.calls))
	}
	if tools.calls[0].Name != "sandbox_exec" {
		t.Fatalf("tool name = %q, want sandbox_exec", tools.calls[0].Name)
	}
	assertTurn(t, result.AgentTurns, 0, wantTurn{
		index:      1,
		status:     run.AgentTurnStatusToolCall,
		actionType: run.AgentTurnActionTypeToolCall,
		toolName:   "sandbox_exec",
		message:    "model requested tool call",
	})
	assertTurn(t, result.AgentTurns, 1, wantTurn{
		index:      2,
		status:     run.AgentTurnStatusFinal,
		actionType: run.AgentTurnActionTypeFinal,
	})
}

func TestRunnerRejectsSandboxExecStaticOutputCommandAndContinues(t *testing.T) {
	staticCommand := `awk 'BEGIN { print "The equation x^5 - x + 1 = 0 has no real solutions." }'`
	computeCommand := `awk 'BEGIN { print (123 * 321) }'`
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{
			{Content: `{"type":"tool_call","tool":"sandbox_exec","arguments":{"command":"` + strings.ReplaceAll(staticCommand, `"`, `\"`) + `","max_output_bytes":65536,"timeout_seconds":10}}`},
			{Content: `{"type":"final","summary":"The equation x^5 - x + 1 = 0 has no real solutions."}`},
			{Content: `{"type":"tool_call","tool":"sandbox_exec","arguments":{"command":"` + computeCommand + `","max_output_bytes":65536,"timeout_seconds":10}}`},
			{Content: `{"type":"final","summary":"39483"}`},
		},
	}
	tools := newFakeToolRunnerWithTools([]outbound.ToolDefinition{
		{Name: "sandbox_exec", Description: "Runs a command inside the sandbox from /workspace/work."},
	})
	tools.results = map[string]outbound.ToolResult{
		"sandbox_exec": {Content: "exit_code: 0\nstdout:\n39483\n"},
	}
	runner := NewRunner(model, tools)

	result, err := runner.Run(context.Background(), RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "solve exactly"},
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
	if result.Summary != "39483" {
		t.Fatalf("summary = %q, want 39483", result.Summary)
	}
	if result.ToolCallCount != 1 {
		t.Fatalf("ToolCallCount = %d, want 1", result.ToolCallCount)
	}
	if len(tools.calls) != 1 {
		t.Fatalf("len(tool calls) = %d, want only corrected command", len(tools.calls))
	}
	if tools.calls[0].Arguments["command"] != computeCommand {
		t.Fatalf("sandbox_exec command = %q, want computed command", tools.calls[0].Arguments["command"])
	}
	assertTurn(t, result.AgentTurns, 0, wantTurn{
		index:      1,
		status:     run.AgentTurnStatusInvalidToolCall,
		actionType: run.AgentTurnActionTypeToolCall,
		toolName:   "sandbox_exec",
		message:    "sandbox_exec command only printed static text",
	})
	assertTurn(t, result.AgentTurns, 1, wantTurn{
		index:              2,
		status:             run.AgentTurnStatusProtocolError,
		actionType:         run.AgentTurnActionTypeFinal,
		message:            "final answer attempted after invalid sandbox_exec",
		correctionContains: "Call sandbox_exec again",
		responsePreview:    "The equation x^5 - x + 1 = 0 has no real solutions.",
	})
	assertTurn(t, result.AgentTurns, 2, wantTurn{
		index:      3,
		status:     run.AgentTurnStatusToolCall,
		actionType: run.AgentTurnActionTypeToolCall,
		toolName:   "sandbox_exec",
	})
	correction := requestMessages(model.requests[1])
	assertMessage(t, correction[len(correction)-1], "runtime", "only printed static text")
	assertMessage(t, correction[len(correction)-1], "runtime", "performs the calculation")
	finalCorrection := requestMessages(model.requests[2])
	assertMessage(t, finalCorrection[len(finalCorrection)-1], "runtime", "only printed static text")
}

func TestSandboxExecCommandOnlyPrintsStaticText(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    bool
	}{
		{
			name:    "awk static sentence",
			command: `awk 'BEGIN { print "The equation has no real solutions." }'`,
			want:    true,
		},
		{
			name:    "echo static sentence",
			command: `echo "The answer is 42"`,
			want:    true,
		},
		{
			name:    "printf static sentence",
			command: `printf "The answer is 42\n"`,
			want:    true,
		},
		{
			name:    "python static sentence",
			command: `python3 -c 'print("The answer is 42")'`,
			want:    true,
		},
		{
			name:    "awk arithmetic",
			command: `awk 'BEGIN { print (123 * 321) }'`,
			want:    false,
		},
		{
			name:    "awk label and arithmetic",
			command: `awk 'BEGIN { print "root=", sqrt(2) }'`,
			want:    false,
		},
		{
			name:    "printf shell arithmetic",
			command: `printf '%s\n' "$((123 * 321))"`,
			want:    false,
		},
		{
			name:    "python arithmetic",
			command: `python3 -c 'print(123 * 321)'`,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sandboxExecCommandOnlyPrintsStaticText(toolNameSandboxExec, map[string]string{"command": tt.command})
			if got != tt.want {
				t.Fatalf("sandboxExecCommandOnlyPrintsStaticText() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestRunnerRejectsUnverifiedNumericalSolveAndContinues(t *testing.T) {
	unverifiedCommand := `python3 -c 'from scipy.optimize import fsolve; f=lambda x:x**5-x+1; root=fsolve(f,0); print(root[0])'`
	verifiedCommand := `python3 -c 'from scipy.optimize import brentq; f=lambda x:x**5-x+1; x=brentq(f,-2,-1); print("root=", x, "residual=", f(x))'`
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{
			{Content: `{"type":"tool_call","tool":"sandbox_exec","arguments":{"command":"` + strings.ReplaceAll(unverifiedCommand, `"`, `\"`) + `","max_output_bytes":65536,"timeout_seconds":10}}`},
			{Content: `{"type":"final","summary":"x is approximately 0.6687327712886604."}`},
			{Content: `{"type":"tool_call","tool":"sandbox_exec","arguments":{"command":"` + strings.ReplaceAll(verifiedCommand, `"`, `\"`) + `","max_output_bytes":65536,"timeout_seconds":10}}`},
			{Content: `{"type":"final","summary":"x is approximately -1.1673039782614187."}`},
		},
	}
	tools := newFakeToolRunnerWithTools([]outbound.ToolDefinition{
		{Name: "sandbox_exec", Description: "Runs a command inside the sandbox from /workspace/work."},
	})
	tools.results = map[string]outbound.ToolResult{
		"sandbox_exec": {Content: "exit_code: 0\nstdout:\nroot= -1.1673039782614187 residual= 0.0\n"},
	}
	runner := NewRunner(model, tools)

	result, err := runner.Run(context.Background(), RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "solve x^5 - x + 1 = 0"},
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
	if result.Summary != "x is approximately -1.1673039782614187." {
		t.Fatalf("summary = %q, want corrected root", result.Summary)
	}
	if result.ToolCallCount != 1 {
		t.Fatalf("ToolCallCount = %d, want only verified command", result.ToolCallCount)
	}
	if len(tools.calls) != 1 {
		t.Fatalf("len(tool calls) = %d, want only verified command", len(tools.calls))
	}
	if tools.calls[0].Arguments["command"] != verifiedCommand {
		t.Fatalf("sandbox_exec command = %q, want verified command", tools.calls[0].Arguments["command"])
	}
	assertTurn(t, result.AgentTurns, 0, wantTurn{
		index:      1,
		status:     run.AgentTurnStatusInvalidToolCall,
		actionType: run.AgentTurnActionTypeToolCall,
		toolName:   "sandbox_exec",
		message:    "sandbox_exec numerical solve did not verify residual",
	})
	assertTurn(t, result.AgentTurns, 1, wantTurn{
		index:              2,
		status:             run.AgentTurnStatusProtocolError,
		actionType:         run.AgentTurnActionTypeFinal,
		message:            "final answer attempted after unverified sandbox_exec numerical solve",
		correctionContains: "bracketed method",
		responsePreview:    "x is approximately 0.6687327712886604.",
	})
	assertTurn(t, result.AgentTurns, 2, wantTurn{
		index:      3,
		status:     run.AgentTurnStatusToolCall,
		actionType: run.AgentTurnActionTypeToolCall,
		toolName:   "sandbox_exec",
	})
	correction := requestMessages(model.requests[1])
	assertMessage(t, correction[len(correction)-1], "runtime", "unverified numerical solve")
	assertMessage(t, correction[len(correction)-1], "runtime", "residual f(root)")
	finalCorrection := requestMessages(model.requests[2])
	assertMessage(t, finalCorrection[len(finalCorrection)-1], "runtime", "unverified numerical solve")
}

func TestSandboxExecCommandUsesUnverifiedNumericalSolve(t *testing.T) {
	tests := []struct {
		name    string
		tool    string
		command string
		want    bool
	}{
		{
			name:    "fsolve candidate only",
			tool:    toolNameSandboxExec,
			command: `python3 -c 'from scipy.optimize import fsolve; f=lambda x:x**5-x+1; root=fsolve(f,0); print(root[0])'`,
			want:    true,
		},
		{
			name:    "optimize root candidate only",
			tool:    toolNameSandboxExec,
			command: `python3 -c 'import scipy.optimize as optimize; result=optimize.root(lambda x: x**2-2, 1); print(result.x[0])'`,
			want:    true,
		},
		{
			name:    "brentq bracketed root",
			tool:    toolNameSandboxExec,
			command: `python3 -c 'from scipy.optimize import brentq; print(brentq(lambda x:x*x-2,1,2))'`,
			want:    false,
		},
		{
			name:    "brentq with residual",
			tool:    toolNameSandboxExec,
			command: `python3 -c 'from scipy.optimize import brentq; f=lambda x:x*x-2; x=brentq(f,1,2); print("root=", x, "residual=", f(x))'`,
			want:    false,
		},
		{
			name:    "fsolve with residual",
			tool:    toolNameSandboxExec,
			command: `python3 -c 'from scipy.optimize import fsolve; f=lambda x:x*x-2; root=fsolve(f,1)[0]; print(root, f(root))'`,
			want:    false,
		},
		{
			name:    "manual bisection",
			tool:    toolNameSandboxExec,
			command: `python3 -c 'print("root=", -1.167, "residual=", 0.0)'`,
			want:    false,
		},
		{
			name:    "different tool",
			tool:    "workspace",
			command: `python3 -c 'from scipy.optimize import fsolve; print(fsolve(lambda x:x, 0)[0])'`,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sandboxExecCommandUsesUnverifiedNumericalSolve(tt.tool, map[string]string{"command": tt.command})
			if got != tt.want {
				t.Fatalf("sandboxExecCommandUsesUnverifiedNumericalSolve() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestRunnerRejectsPDFToTextDefaultOutputInReadOnlyInputAndContinues(t *testing.T) {
	badCommand := `pdftotext /workspace/input/manual.pdf | grep -i 'keyword'`
	correctedCommand := `pdftotext '/workspace/input/manual.pdf' - | sed -n '1,40p'`
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{
			{Content: `{"type":"tool_call","tool":"sandbox_exec","arguments":{"command":"` + strings.ReplaceAll(badCommand, `"`, `\"`) + `","max_output_bytes":65536}}`},
			{Content: `{"type":"final","summary":"Use the steamer setting."}`},
			{Content: `{"type":"tool_call","tool":"sandbox_exec","arguments":{"command":"` + strings.ReplaceAll(correctedCommand, `"`, `\"`) + `","max_output_bytes":65536}}`},
			{Content: `{"type":"final","summary":"Use the steamer setting, based on manual page 12."}`},
		},
	}
	tools := newFakeToolRunnerWithTools([]outbound.ToolDefinition{
		{Name: "sandbox_exec", Description: "Runs a command inside the sandbox from /workspace/work."},
	})
	tools.results = map[string]outbound.ToolResult{
		"sandbox_exec": {Content: "exit_code: 0\nstdout:\npage 12 steamer setting\n"},
	}
	runner := NewRunner(model, tools)

	result, err := runner.Run(context.Background(), RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "find cooking instructions in the manuals"},
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
	if result.Summary != "Use the steamer setting, based on manual page 12." {
		t.Fatalf("summary = %q, want corrected final", result.Summary)
	}
	if result.ToolCallCount != 1 {
		t.Fatalf("ToolCallCount = %d, want only corrected command", result.ToolCallCount)
	}
	if len(tools.calls) != 1 {
		t.Fatalf("len(tool calls) = %d, want only corrected command", len(tools.calls))
	}
	if tools.calls[0].Arguments["command"] != correctedCommand {
		t.Fatalf("sandbox_exec command = %q, want corrected command", tools.calls[0].Arguments["command"])
	}
	assertTurn(t, result.AgentTurns, 0, wantTurn{
		index:      1,
		status:     run.AgentTurnStatusInvalidToolCall,
		actionType: run.AgentTurnActionTypeToolCall,
		toolName:   "sandbox_exec",
		message:    "sandbox_exec pdftotext command would write beside read-only input",
	})
	assertTurn(t, result.AgentTurns, 1, wantTurn{
		index:              2,
		status:             run.AgentTurnStatusProtocolError,
		actionType:         run.AgentTurnActionTypeFinal,
		message:            "final answer attempted after invalid sandbox_exec pdftotext command",
		correctionContains: "pdftotext",
		responsePreview:    "Use the steamer setting.",
	})
	assertTurn(t, result.AgentTurns, 2, wantTurn{
		index:      3,
		status:     run.AgentTurnStatusToolCall,
		actionType: run.AgentTurnActionTypeToolCall,
		toolName:   "sandbox_exec",
	})
	correction := requestMessages(model.requests[1])
	assertMessage(t, correction[len(correction)-1], "runtime", "read-only /workspace/input PDF")
	assertMessage(t, correction[len(correction)-1], "runtime", "create your own small script under /workspace/work")
	finalCorrection := requestMessages(model.requests[2])
	assertMessage(t, finalCorrection[len(finalCorrection)-1], "runtime", "read-only /workspace/input PDF")
}

func TestSandboxExecCommandWritesReadOnlyPDFTextOutput(t *testing.T) {
	tests := []struct {
		name    string
		tool    string
		command string
		want    bool
	}{
		{
			name:    "pdftotext default output next to input",
			tool:    toolNameSandboxExec,
			command: `pdftotext /workspace/input/manual.pdf | grep -i keyword`,
			want:    true,
		},
		{
			name:    "quoted input path default output",
			tool:    toolNameSandboxExec,
			command: `pdftotext '/workspace/input/manual with spaces.pdf'`,
			want:    true,
		},
		{
			name:    "option then default output",
			tool:    toolNameSandboxExec,
			command: `pdftotext -layout /workspace/input/manual.pdf | grep -i keyword`,
			want:    true,
		},
		{
			name:    "stdout output",
			tool:    toolNameSandboxExec,
			command: `pdftotext '/workspace/input/manual.pdf' - | grep -i keyword`,
			want:    false,
		},
		{
			name:    "workspace work output",
			tool:    toolNameSandboxExec,
			command: `pdftotext '/workspace/input/manual.pdf' /workspace/work/manual.txt`,
			want:    false,
		},
		{
			name:    "relative output in workdir",
			tool:    toolNameSandboxExec,
			command: `pdftotext '/workspace/input/manual.pdf' manual.txt`,
			want:    false,
		},
		{
			name:    "explicit output under read only input",
			tool:    toolNameSandboxExec,
			command: `pdftotext '/workspace/input/manual.pdf' /workspace/input/manual.txt`,
			want:    true,
		},
		{
			name:    "redirection does not change pdftotext default output",
			tool:    toolNameSandboxExec,
			command: `pdftotext /workspace/input/manual.pdf > manual.txt`,
			want:    true,
		},
		{
			name:    "non input pdf",
			tool:    toolNameSandboxExec,
			command: `pdftotext /workspace/work/manual.pdf | grep -i keyword`,
			want:    false,
		},
		{
			name:    "different tool",
			tool:    "workspace",
			command: `pdftotext /workspace/input/manual.pdf | grep -i keyword`,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sandboxExecCommandWritesReadOnlyPDFTextOutput(tt.tool, map[string]string{"command": tt.command})
			if got != tt.want {
				t.Fatalf("sandboxExecCommandWritesReadOnlyPDFTextOutput() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestSandboxExecCommandDumpsPDFTextToStdout(t *testing.T) {
	tests := []struct {
		name    string
		tool    string
		command string
		want    bool
	}{
		{
			name:    "full pdf stdout",
			tool:    toolNameSandboxExec,
			command: `pdftotext /workspace/input/manual.pdf -`,
			want:    true,
		},
		{
			name:    "full pdf stdout with stderr redirection",
			tool:    toolNameSandboxExec,
			command: `pdftotext /workspace/input/manual.pdf - 2>/dev/null`,
			want:    true,
		},
		{
			name:    "grep narrows stdout",
			tool:    toolNameSandboxExec,
			command: `pdftotext /workspace/input/manual.pdf - 2>/dev/null | grep -nEi 'steam|cook' | head -20`,
			want:    false,
		},
		{
			name:    "sed narrows stdout",
			tool:    toolNameSandboxExec,
			command: `pdftotext /workspace/input/manual.pdf - 2>/dev/null | sed -n '10,20p'`,
			want:    false,
		},
		{
			name:    "output file under work",
			tool:    toolNameSandboxExec,
			command: `pdftotext /workspace/input/manual.pdf /workspace/work/manual.txt`,
			want:    false,
		},
		{
			name:    "non sandbox tool",
			tool:    "workspace",
			command: `pdftotext /workspace/input/manual.pdf -`,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sandboxExecCommandDumpsPDFTextToStdout(tt.tool, map[string]string{"command": tt.command})
			if got != tt.want {
				t.Fatalf("sandboxExecCommandDumpsPDFTextToStdout() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestRunnerRequiresSuccessfulSandboxExecAfterSandboxErrorBeforeFinal(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{
			{Content: `{"type":"tool_call","tool":"sandbox_exec","arguments":{"command":"bad math"}}`},
			{Content: `{"type":"final","summary":"x = 7 or x = 9"}`},
			{Content: `{"type":"tool_call","tool":"sandbox_exec","arguments":{"command":"awk 'BEGIN { print 7; print 9 }'"}}`},
			{Content: `{"type":"final","summary":"x = 7 or x = 9"}`},
		},
	}
	tools := newFakeToolRunnerWithTools([]outbound.ToolDefinition{
		{Name: "sandbox_exec", Description: "Runs a command inside the sandbox from /workspace/work."},
	})
	tools.resultQueue = []outbound.ToolResult{
		{Content: "exit_code: 2\nstderr:\n/bin/sh: arithmetic syntax error\n", IsError: true},
		{Content: "exit_code: 0\nstdout:\n7\n9\n"},
	}
	runner := NewRunner(model, tools)

	result, err := runner.Run(context.Background(), RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "請幫我解這個一元二次方程式，x^2-16x+63=0 求x?"},
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
	if result.Summary != "x = 7 or x = 9" {
		t.Fatalf("summary = %q, want verified final", result.Summary)
	}
	if result.ToolCallCount != 2 {
		t.Fatalf("ToolCallCount = %d, want 2", result.ToolCallCount)
	}
	if len(result.ToolCalls) != 2 {
		t.Fatalf("len(ToolCalls) = %d, want 2", len(result.ToolCalls))
	}
	if !result.ToolCalls[0].IsError {
		t.Fatal("first sandbox_exec IsError = false, want true")
	}
	if result.ToolCalls[1].IsError {
		t.Fatal("second sandbox_exec IsError = true, want false")
	}
	if len(tools.calls) != 2 {
		t.Fatalf("len(tool calls) = %d, want 2", len(tools.calls))
	}
	assertTurn(t, result.AgentTurns, 0, wantTurn{
		index:      1,
		status:     run.AgentTurnStatusToolCall,
		actionType: run.AgentTurnActionTypeToolCall,
		toolName:   "sandbox_exec",
	})
	assertTurn(t, result.AgentTurns, 1, wantTurn{
		index:              2,
		status:             run.AgentTurnStatusProtocolError,
		actionType:         run.AgentTurnActionTypeFinal,
		message:            "final answer attempted after failed sandbox_exec",
		correctionContains: "Call sandbox_exec again with a corrected command before returning final.",
		responsePreview:    "x = 7 or x = 9",
	})
	assertTurn(t, result.AgentTurns, 2, wantTurn{
		index:      3,
		status:     run.AgentTurnStatusToolCall,
		actionType: run.AgentTurnActionTypeToolCall,
		toolName:   "sandbox_exec",
	})
	assertTurn(t, result.AgentTurns, 3, wantTurn{
		index:      4,
		status:     run.AgentTurnStatusFinal,
		actionType: run.AgentTurnActionTypeFinal,
	})
	if len(model.requests) != 4 {
		t.Fatalf("len(model requests) = %d, want 4", len(model.requests))
	}
	correction := requestMessages(model.requests[2])[len(requestMessages(model.requests[2]))-1]
	assertMessage(t, correction, "runtime", "previous sandbox_exec command failed")
}

func TestRunnerRejectsRepeatedFailedSandboxExecCommandAndContinues(t *testing.T) {
	failedCommand := `pdftotext /workspace/input/manual.pdf - | grep 'exact phrase'`
	correctedCommand := `pdftotext /workspace/input/manual.pdf - 2>/dev/null | sed -n '1,80p'`
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{
			{Content: `{"type":"tool_call","tool":"sandbox_exec","arguments":{"command":"` + strings.ReplaceAll(failedCommand, `"`, `\"`) + `"}}`},
			{Content: `{"type":"tool_call","tool":"sandbox_exec","arguments":{"command":"` + strings.ReplaceAll(failedCommand, `"`, `\"`) + `"}}`},
			{Content: `{"type":"final","summary":"Use the steamer setting."}`},
			{Content: `{"type":"tool_call","tool":"sandbox_exec","arguments":{"command":"` + strings.ReplaceAll(correctedCommand, `"`, `\"`) + `"}}`},
			{Content: `{"type":"final","summary":"Use the steamer setting, based on manual page 12."}`},
		},
	}
	tools := newFakeToolRunnerWithTools([]outbound.ToolDefinition{
		{Name: "sandbox_exec", Description: "Runs a command inside the sandbox from /workspace/work."},
	})
	tools.resultQueue = []outbound.ToolResult{
		{Content: "exit_code: 1\nstdout:\n\nstderr:\npdftotext warnings omitted: 120 line(s)\n", IsError: true},
		{Content: "exit_code: 0\nstdout:\npage 12 steamer setting\n"},
	}
	runner := NewRunner(model, tools)

	result, err := runner.Run(context.Background(), RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "find cooking instructions in the manuals"},
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
	if result.Summary != "Use the steamer setting, based on manual page 12." {
		t.Fatalf("summary = %q, want corrected final", result.Summary)
	}
	if len(tools.calls) != 2 {
		t.Fatalf("len(tool calls) = %d, want failed and corrected commands only", len(tools.calls))
	}
	if tools.calls[0].Arguments["command"] != failedCommand {
		t.Fatalf("first command = %q, want failed command", tools.calls[0].Arguments["command"])
	}
	if tools.calls[1].Arguments["command"] != correctedCommand {
		t.Fatalf("second command = %q, want corrected command", tools.calls[1].Arguments["command"])
	}
	assertTurn(t, result.AgentTurns, 1, wantTurn{
		index:      2,
		status:     run.AgentTurnStatusInvalidToolCall,
		actionType: run.AgentTurnActionTypeToolCall,
		toolName:   "sandbox_exec",
		message:    "sandbox_exec repeated a failed command unchanged",
	})
	assertTurn(t, result.AgentTurns, 2, wantTurn{
		index:              3,
		status:             run.AgentTurnStatusProtocolError,
		actionType:         run.AgentTurnActionTypeFinal,
		message:            "final answer attempted after repeated failed sandbox_exec command",
		correctionContains: "materially corrected command",
		responsePreview:    "Use the steamer setting.",
	})
	correction := requestMessages(model.requests[2])
	assertMessage(t, correction[len(correction)-1], "runtime", "exact-string-only loops")
	assertMessage(t, correction[len(correction)-1], "runtime", "small /workspace/work script")
}

func TestRunnerRejectsRepeatedSuccessfulSandboxExecCommandAndAllowsFinal(t *testing.T) {
	command := `python3 -c 'print(6 * 7)'`
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{
			{Content: `{"type":"tool_call","tool":"sandbox_exec","arguments":{"command":"` + strings.ReplaceAll(command, `"`, `\"`) + `"}}`},
			{Content: `{"type":"tool_call","tool":"sandbox_exec","arguments":{"command":"` + strings.ReplaceAll(command, `"`, `\"`) + `"}}`},
			{Content: `{"type":"final","summary":"42"}`},
		},
	}
	tools := newFakeToolRunnerWithTools([]outbound.ToolDefinition{
		{Name: "sandbox_exec", Description: "Runs a command inside the sandbox from /workspace/work."},
	})
	tools.resultQueue = []outbound.ToolResult{
		{Content: "exit_code: 0\nstdout:\n42\n"},
	}
	runner := NewRunner(model, tools)

	result, err := runner.Run(context.Background(), RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "find cooking instructions in the manuals"},
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
	if result.Summary != "42" {
		t.Fatalf("summary = %q, want final based on first tool result", result.Summary)
	}
	if len(tools.calls) != 1 {
		t.Fatalf("len(tool calls) = %d, want repeated successful command to be blocked", len(tools.calls))
	}
	assertTurn(t, result.AgentTurns, 1, wantTurn{
		index:      2,
		status:     run.AgentTurnStatusInvalidToolCall,
		actionType: run.AgentTurnActionTypeToolCall,
		toolName:   "sandbox_exec",
		message:    "sandbox_exec repeated a successful command unchanged",
	})
	assertTurn(t, result.AgentTurns, 2, wantTurn{
		index:      3,
		status:     run.AgentTurnStatusFinal,
		actionType: run.AgentTurnActionTypeFinal,
	})
	correction := requestMessages(model.requests[2])
	assertMessage(t, correction[len(correction)-1], "runtime", "already succeeded")
	assertMessage(t, correction[len(correction)-1], "runtime", "final answer, not a tool_call")
}

func TestRunnerRequiresNearbyContextAfterPDFSearchHitsBeforeFinal(t *testing.T) {
	searchCommand := `pdftotext /workspace/input/manual.pdf - 2>/dev/null | grep -nEi 'steam|dumpling|cook' | head -20`
	contextCommand := `pdftotext /workspace/input/manual.pdf - 2>/dev/null | sed -n '500,540p'`
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{
			{Content: `{"type":"tool_call","tool":"sandbox_exec","arguments":{"command":"` + strings.ReplaceAll(searchCommand, `"`, `\"`) + `"}}`},
			{Content: `{"type":"final","summary":"The cooking method can be found in manual.pdf line 520."}`},
			{Content: `{"type":"tool_call","tool":"sandbox_exec","arguments":{"command":"` + strings.ReplaceAll(contextCommand, `"`, `\"`) + `"}}`},
			{Content: `{"type":"final","summary":"Steam the dumplings, based on manual.pdf near line 520."}`},
		},
	}
	tools := newFakeToolRunnerWithTools([]outbound.ToolDefinition{
		{Name: "sandbox_exec", Description: "Runs a command inside the sandbox from /workspace/work."},
	})
	tools.resultQueue = []outbound.ToolResult{
		{Content: "exit_code: 0\nstdout:\n520:steam frozen dumplings\nstderr:\n"},
		{Content: "exit_code: 0\nstdout:\nUse steam mode for frozen dumplings.\n"},
	}
	runner := NewRunner(model, tools)

	result, err := runner.Run(context.Background(), RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "find cooking instructions in the manuals"},
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
	if result.Summary != "Steam the dumplings, based on manual.pdf near line 520." {
		t.Fatalf("summary = %q, want context-based final", result.Summary)
	}
	if len(tools.calls) != 2 {
		t.Fatalf("len(tool calls) = %d, want search and context commands", len(tools.calls))
	}
	assertTurn(t, result.AgentTurns, 1, wantTurn{
		index:              2,
		status:             run.AgentTurnStatusProtocolError,
		actionType:         run.AgentTurnActionTypeFinal,
		message:            "final answer attempted after PDF search hits without nearby context",
		correctionContains: "nearby context",
		responsePreview:    "The cooking method can be found in manual.pdf line 520.",
	})
	assertTurn(t, result.AgentTurns, 3, wantTurn{
		index:      4,
		status:     run.AgentTurnStatusFinal,
		actionType: run.AgentTurnActionTypeFinal,
	})
	correction := requestMessages(model.requests[2])
	assertMessage(t, correction[len(correction)-1], "runtime", "search hit lines only")
}

func TestRunnerRejectsFinalThatIgnoresRequestedCJKLanguage(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{
			{Content: `{"type":"final","summary":"The answer is in English."}`},
			{Content: `{"type":"final","summary":"這是中文回答。"}`},
		},
	}
	runner := NewRunner(model, newFakeToolRunnerWithTools(nil))

	result, err := runner.Run(context.Background(), RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "請用中文回答"},
	})
	if err != nil {
		t.Fatalf("run agent: %v", err)
	}
	if result.Summary != "這是中文回答。" {
		t.Fatalf("summary = %q, want corrected Chinese final", result.Summary)
	}
	assertTurn(t, result.AgentTurns, 0, wantTurn{
		index:              1,
		status:             run.AgentTurnStatusProtocolError,
		actionType:         run.AgentTurnActionTypeFinal,
		message:            "final answer did not preserve requested language",
		correctionContains: "requested language",
		responsePreview:    "The answer is in English.",
	})
	assertTurn(t, result.AgentTurns, 1, wantTurn{
		index:      2,
		status:     run.AgentTurnStatusFinal,
		actionType: run.AgentTurnActionTypeFinal,
	})
}

func TestBuildInitialTurnsAddsChineseResponseLanguageForCJKPrompt(t *testing.T) {
	turns := buildInitialTurns(run.TaskSpec{Prompt: "請讀文件"})
	if len(turns) != 1 || len(turns[0].Parts) == 0 {
		t.Fatalf("turns = %#v, want initial user turn", turns)
	}
	got := turns[0].Parts[0].Text
	if !strings.Contains(got, "Response language: 請用中文回答 final.summary") {
		t.Fatalf("prompt = %q, want Chinese response language guidance", got)
	}
}

func TestBuildInitialTurnsDoesNotAddChineseResponseLanguageForNonCJKPrompt(t *testing.T) {
	turns := buildInitialTurns(run.TaskSpec{Prompt: "Read the file"})
	if got := turns[0].Parts[0].Text; strings.Contains(got, "Response language:") {
		t.Fatalf("prompt = %q, want no extra response language guidance", got)
	}
}

func TestRunnerCallsNativeToolAndReturnsFinalSummary(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{
			{
				ToolCalls: []outbound.ModelToolCall{
					{ID: "call_echo_1", Name: "echo", Arguments: map[string]string{"text": "hello"}},
				},
			},
			{Content: `{"type":"final","summary":"echoed hello"}`},
		},
	}
	tools := newFakeToolRunner()
	runner := NewRunner(
		model,
		tools,
		WithClock(sequenceClock(
			timeUnix(101),
			timeUnix(102),
			timeUnix(103),
			timeUnix(104),
			timeUnix(105),
			timeUnix(106),
		)),
	)

	result, err := runner.Run(context.Background(), RunRequest{
		RunID: "run_test",
		Task:  run.TaskSpec{Prompt: "echo hello"},
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
	assertEchoToolInvocation(t, tools)
	assertTurn(t, result.AgentTurns, 0, wantTurn{
		index:      1,
		status:     run.AgentTurnStatusToolCall,
		actionType: run.AgentTurnActionTypeToolCall,
		toolName:   "echo",
		message:    "model requested native tool call",
		startedAt:  timeUnix(101),
		endedAt:    timeUnix(102),
	})
	if !strings.Contains(result.AgentTurns[0].RawResponse, `"id":"call_echo_1"`) {
		t.Fatalf("native raw response = %q, want tool call id", result.AgentTurns[0].RawResponse)
	}
	if len(model.requests[1].Turns) < 3 {
		t.Fatalf("len(second request turns) = %d, want task, assistant tool call, and tool result", len(model.requests[1].Turns))
	}
	lastMessages := requestMessages(model.requests[1])
	assertMessage(t, lastMessages[len(lastMessages)-2], "assistant", "")
	if lastMessages[len(lastMessages)-2].Kind != string(outbound.ModelPartKindToolCall) {
		t.Fatalf("assistant native part kind = %q, want tool_call", lastMessages[len(lastMessages)-2].Kind)
	}
	if lastMessages[len(lastMessages)-2].ToolCallID != "call_echo_1" {
		t.Fatalf("assistant native tool_call_id = %q, want call_echo_1", lastMessages[len(lastMessages)-2].ToolCallID)
	}
	assertMessage(t, lastMessages[len(lastMessages)-1], "tool", "Tool result for echo:\nhello")
	if lastMessages[len(lastMessages)-1].ToolCallID != "call_echo_1" {
		t.Fatalf("tool result tool_call_id = %q, want call_echo_1", lastMessages[len(lastMessages)-1].ToolCallID)
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
	lastMessages := requestMessages(model.requests[1])
	assertMessage(t, lastMessages[len(lastMessages)-1], "runtime", `The tool "sh_script" is not available.`)
	assertMessage(t, lastMessages[len(lastMessages)-1], "runtime", "Available tools: none")
	assertMessage(t, lastMessages[len(lastMessages)-1], "runtime", "Do not invent tool names.")
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
	lastMessages := requestMessages(model.requests[1])
	assertMessage(t, lastMessages[len(lastMessages)-1], "tool", "Tool error for workspace:\npath is not available")
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
	correction := requestMessages(model.requests[1])[len(requestMessages(model.requests[1]))-1]
	assertMessage(t, correction, "runtime", "placeholder argument values: command=<file_path>")
	assertMessage(t, correction, "runtime", "Uploaded files: README.md")
	assertMessage(t, correction, "runtime", "Available tools: sandbox_exec, workspace")
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
	lastMessages := requestMessages(model.requests[1])
	if len(lastMessages) != 4 {
		t.Fatalf("len(second request messages) = %d, want instructions, task, assistant attempt, and correction", len(lastMessages))
	}
	assertMessage(t, lastMessages[len(lastMessages)-2], "assistant", "tool_result")
	if lastMessages[len(lastMessages)-2].Kind != string(outbound.ModelPartKindAssistantAttempt) {
		t.Fatalf("assistant attempt kind = %q, want %q", lastMessages[len(lastMessages)-2].Kind, outbound.ModelPartKindAssistantAttempt)
	}
	assertMessage(t, lastMessages[len(lastMessages)-1], "runtime", "Protocol error:")
	assertMessage(t, lastMessages[len(lastMessages)-1], "runtime", "Error code: unknown_action_type")
	assertMessage(t, lastMessages[len(lastMessages)-1], "runtime", "action type must be final or tool_call")
	assertMessage(t, lastMessages[len(lastMessages)-1], "runtime", "Do not add labels such as Final:")
	if strings.Contains(lastMessages[len(lastMessages)-1].Content, "hello from tool") {
		t.Fatalf("protocol correction included invalid raw response content: %q", lastMessages[len(lastMessages)-1].Content)
	}
	if len(tools.calls) != 0 {
		t.Fatalf("len(tool calls) = %d, want 0", len(tools.calls))
	}
	assertTurn(t, result.AgentTurns, 0, wantTurn{
		index:             1,
		status:            run.AgentTurnStatusProtocolError,
		message:           "action type must be final or tool_call",
		rawResponse:       `{"type":"tool_result","result":"hello from tool"}`,
		responseFormat:    modelResponseFormatJSONObject,
		protocolErrorCode: actionParseCodeUnknownActionType,
		responsePreview:   `{"type":"tool_result","result":"hello from tool"}`,
	})
	assertTurn(t, result.AgentTurns, 1, wantTurn{
		index:               2,
		status:              run.AgentTurnStatusFinal,
		actionType:          run.AgentTurnActionTypeFinal,
		requestMessageCount: 4,
		requestContains:     "Error code: unknown_action_type",
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
	lastMessages := requestMessages(model.requests[1])
	assertMessage(t, lastMessages[len(lastMessages)-2], "assistant", `"type":"tool_call"`)
	assertMessage(t, lastMessages[len(lastMessages)-1], "runtime", "multiple JSON objects")
	assertTurn(t, result.AgentTurns, 0, wantTurn{
		index:             1,
		status:            run.AgentTurnStatusProtocolError,
		message:           "model response must contain exactly one JSON object",
		rawResponse:       `{"type":"tool_call","tool":"echo","arguments":{"text":"hello"}}{"type":"final","summary":"bad"}`,
		responseFormat:    modelResponseFormatMultipleJSONValues,
		protocolErrorCode: actionParseCodeMultipleJSONValues,
	})
}

func TestRunnerParsesWholeResponseFencedJSONToolCallAndContinues(t *testing.T) {
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
	if len(tools.calls) != 1 {
		t.Fatalf("len(tool calls) = %d, want 1", len(tools.calls))
	}
	if len(model.requests) != 2 {
		t.Fatalf("len(model requests) = %d, want 2", len(model.requests))
	}
	lastMessages := requestMessages(model.requests[1])
	if len(lastMessages) != 4 {
		t.Fatalf("len(second request messages) = %d, want instructions, task, assistant action, and tool result", len(lastMessages))
	}
	assertMessage(t, lastMessages[len(lastMessages)-2], "assistant", `"type":"tool_call"`)
	assertMessage(t, lastMessages[len(lastMessages)-1], "tool", "Tool result for echo:\nhello")
	assertTurn(t, result.AgentTurns, 0, wantTurn{
		index:           1,
		status:          run.AgentTurnStatusToolCall,
		actionType:      run.AgentTurnActionTypeToolCall,
		toolName:        "echo",
		message:         "model requested tool call",
		rawResponse:     "```json\n{\"type\":\"tool_call\",\"tool\":\"echo\",\"arguments\":{\"text\":\"hello\"}}\n```",
		responseFormat:  modelResponseFormatMarkdownFence,
		responsePreview: "```json\n{\"type\":\"tool_call\",\"tool\":\"echo\",\"arguments\":{\"text\":\"hello\"}}\n```",
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
	lastMessages := requestMessages(model.requests[1])
	assertMessage(t, lastMessages[len(lastMessages)-1], "runtime", "final.summary must be a string, boolean, or number")
	assertTurn(t, result.AgentTurns, 0, wantTurn{
		index:             1,
		status:            run.AgentTurnStatusProtocolError,
		message:           "final.summary must be a string, boolean, or number",
		rawResponse:       `{"type":"final","summary":{"value":"done"}}`,
		responseFormat:    modelResponseFormatJSONObject,
		protocolErrorCode: actionParseCodeInvalidSummary,
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
		index:             1,
		status:            run.AgentTurnStatusProtocolError,
		rawResponse:       `{"type":"tool_result","result":"hello from tool"}`,
		responseFormat:    modelResponseFormatJSONObject,
		protocolErrorCode: actionParseCodeUnknownActionType,
		responsePreview:   `{"type":"tool_result","result":"hello from tool"}`,
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

func TestRunnerRejectsEmptyModelResponseAndContinues(t *testing.T) {
	model := &recordingModelClient{
		responses: []outbound.ModelResponse{
			{Content: "   "},
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
	assertTurn(t, result.AgentTurns, 0, wantTurn{
		index:             1,
		status:            run.AgentTurnStatusProtocolError,
		message:           "model response was not valid JSON",
		responseFormat:    modelResponseFormatEmpty,
		protocolErrorCode: actionParseCodeInvalidJSON,
	})
	assertTurn(t, result.AgentTurns, 1, wantTurn{
		index:      2,
		status:     run.AgentTurnStatusFinal,
		actionType: run.AgentTurnActionTypeFinal,
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

func TestClassifyModelResponseFormat(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "empty", content: " \n\t", want: modelResponseFormatEmpty},
		{name: "plain text", content: "done", want: modelResponseFormatPlainText},
		{name: "markdown fence", content: "```json\n{}\n```", want: modelResponseFormatMarkdownFence},
		{name: "object", content: `{"type":"final","summary":"done"}`, want: modelResponseFormatJSONObject},
		{name: "array", content: `[]`, want: modelResponseFormatJSONArray},
		{name: "scalar", content: `true`, want: modelResponseFormatJSONScalar},
		{name: "invalid json", content: `{"type":`, want: modelResponseFormatInvalidJSON},
		{name: "multiple values", content: `{"type":"final","summary":"done"}{"type":"final","summary":"again"}`, want: modelResponseFormatMultipleJSONValues},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyModelResponseFormat(tt.content); got != tt.want {
				t.Fatalf("classifyModelResponseFormat() = %q, want %q", got, tt.want)
			}
		})
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
	resultQueue  []outbound.ToolResult
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
	if len(r.resultQueue) > 0 {
		result := r.resultQueue[0]
		r.resultQueue = r.resultQueue[1:]

		return result, nil
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

func requestMessages(request outbound.ModelRequest) []TurnMessageRecord {
	return copyModelRequestMessages(request.Instructions, request.Turns)
}

func assertMessage(t *testing.T, message TurnMessageRecord, role string, contentContains string) {
	t.Helper()

	if message.Role != role {
		t.Fatalf("Role = %q, want %q", message.Role, role)
	}
	if !strings.Contains(message.Content, contentContains) {
		t.Fatalf("Content = %q, want to contain %q", message.Content, contentContains)
	}
}

func assertInstruction(t *testing.T, request outbound.ModelRequest, contentContains string) {
	t.Helper()

	if !strings.Contains(request.Instructions, contentContains) {
		t.Fatalf("Instructions = %q, want to contain %q", request.Instructions, contentContains)
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
	index               int
	status              string
	actionType          string
	toolName            string
	message             string
	requestMessageCount int
	requestContains     string
	rawResponse         string
	responseFormat      string
	protocolErrorCode   string
	correctionContains  string
	responsePreview     string
	startedAt           time.Time
	endedAt             time.Time
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
	if want.requestMessageCount > 0 && len(turn.RequestMessages) != want.requestMessageCount {
		t.Fatalf("AgentTurns[%d].RequestMessages length = %d, want %d", index, len(turn.RequestMessages), want.requestMessageCount)
	}
	if want.requestContains != "" {
		found := false
		for _, message := range turn.RequestMessages {
			if strings.Contains(message.Content, want.requestContains) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("AgentTurns[%d].RequestMessages = %#v, want content containing %q", index, turn.RequestMessages, want.requestContains)
		}
	}
	if want.rawResponse != "" && turn.RawResponse != want.rawResponse {
		t.Fatalf("AgentTurns[%d].RawResponse = %q, want %q", index, turn.RawResponse, want.rawResponse)
	}
	if want.responseFormat != "" && turn.ResponseFormat != want.responseFormat {
		t.Fatalf("AgentTurns[%d].ResponseFormat = %q, want %q", index, turn.ResponseFormat, want.responseFormat)
	}
	if want.protocolErrorCode != "" && turn.ProtocolErrorCode != want.protocolErrorCode {
		t.Fatalf("AgentTurns[%d].ProtocolErrorCode = %q, want %q", index, turn.ProtocolErrorCode, want.protocolErrorCode)
	}
	if want.correctionContains != "" && !strings.Contains(turn.CorrectionMessage, want.correctionContains) {
		t.Fatalf(
			"AgentTurns[%d].CorrectionMessage = %q, want to contain %q",
			index,
			turn.CorrectionMessage,
			want.correctionContains,
		)
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
