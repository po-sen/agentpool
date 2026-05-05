package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	applicationagent "github.com/po-sen/agentpool/internal/application/agent"
	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestWorkerProcessOneCompletesQueuedRun(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	queue := &fakeRunQueue{}
	publisher := &recordingPublisher{}
	now := time.Unix(100, 0).UTC()

	item := queueRun(ctx, t, repo, queue, now)

	worker := newWorker(queue, repo, publisher, now)
	if err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("process one: %v", err)
	}

	stored := findRun(ctx, t, repo, item.ID)
	if stored.Status != run.StatusCompleted {
		t.Fatalf("stored status = %s, want %s", stored.Status, run.StatusCompleted)
	}
	if stored.ResultSummary != "done" {
		t.Fatalf("stored result summary = %q, want %q", stored.ResultSummary, "done")
	}
	if stored.FailureReason != "" {
		t.Fatalf("stored failure reason = %q, want empty", stored.FailureReason)
	}
	if stored.AgentSystemPrompt == "" {
		t.Fatal("stored agent system prompt is empty")
	}
	if !strings.Contains(stored.AgentSystemPrompt, "Available tools:") {
		t.Fatalf("stored agent system prompt = %q, want available tools", stored.AgentSystemPrompt)
	}
	assertAgentTurnStatuses(t, stored.AgentTurns, []string{run.AgentTurnStatusNaturalLanguageFinal})
	assertSteps(t, stored.Steps, []wantStep{
		{
			name:    "prepare",
			status:  run.StatusCompleted,
			message: "Prepared policy, secrets, and source context",
			ended:   true,
		},
		{
			name:    "agent",
			status:  run.StatusCompleted,
			message: "Agent generated result summary",
			ended:   true,
		},
	})

	wantEvents := []string{
		outbound.EventRunPreparing,
		outbound.EventRunStarted,
		outbound.EventRunCompleted,
	}
	if len(publisher.events) != len(wantEvents) {
		t.Fatalf("event count = %d, want %d", len(publisher.events), len(wantEvents))
	}
	for i, want := range wantEvents {
		if publisher.events[i].Type != want {
			t.Fatalf("event[%d] = %s, want %s", i, publisher.events[i].Type, want)
		}
	}
}

func TestWorkerProcessOneRecordsToolCallCountInAgentStep(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	queue := &fakeRunQueue{}
	publisher := &recordingPublisher{}
	now := time.Unix(100, 0).UTC()

	item := queueRun(ctx, t, repo, queue, now)

	worker := newWorkerWithModel(
		queue,
		repo,
		publisher,
		now,
		&toolCallingModelClient{},
	)
	if err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("process one: %v", err)
	}

	stored := findRun(ctx, t, repo, item.ID)
	if stored.ResultSummary != "done with tool" {
		t.Fatalf("stored result summary = %q, want done with tool", stored.ResultSummary)
	}
	if len(stored.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(stored.ToolCalls))
	}
	if stored.ToolCalls[0].Name != "echo" {
		t.Fatalf("tool call name = %q, want echo", stored.ToolCalls[0].Name)
	}
	if stored.ToolCalls[0].Arguments["text"] != "hello" {
		t.Fatalf("tool call text = %q, want hello", stored.ToolCalls[0].Arguments["text"])
	}
	if stored.ToolCalls[0].Result != "tool result" {
		t.Fatalf("tool call result = %q, want tool result", stored.ToolCalls[0].Result)
	}
	assertAgentTurnStatuses(t, stored.AgentTurns, []string{
		run.AgentTurnStatusToolCall,
		run.AgentTurnStatusFinal,
	})
	if stored.AgentTurns[0].ToolName != "echo" {
		t.Fatalf("AgentTurns[0].ToolName = %q, want echo", stored.AgentTurns[0].ToolName)
	}
	assertSteps(t, stored.Steps, []wantStep{
		{
			name:    "prepare",
			status:  run.StatusCompleted,
			message: "Prepared policy, secrets, and source context",
			ended:   true,
		},
		{
			name:    "agent",
			status:  run.StatusCompleted,
			message: "Agent generated result summary after 1 tool call(s)",
			ended:   true,
		},
	})
}

func TestWorkerStoresWorkspaceAndSandboxExecToolCallHistory(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	queue := &fakeRunQueue{}
	publisher := &recordingPublisher{}
	workspace := &recordingWorkspaceProvider{path: "/tmp/workspace"}
	sandbox := &recordingSandboxProvider{
		supportsCommands: true,
		sandbox:          outbound.Sandbox{ID: "sandbox_test", SupportsCommands: true},
	}
	now := time.Unix(100, 0).UTC()

	queueRunWithTask(ctx, t, repo, queue, textAttachmentTask(), now)

	worker := newWorkerWithWorkspaceAndSandbox(
		queue,
		repo,
		publisher,
		now,
		applicationagent.NewRunner(&workspaceAndSandboxToolCallingModelClient{}, fakeToolRunner{}),
		workspace,
		sandbox,
	)
	if err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("process one: %v", err)
	}

	stored := findRun(ctx, t, repo, "run_test")
	if len(stored.ToolCalls) != 2 {
		t.Fatalf("len(ToolCalls) = %d, want 2", len(stored.ToolCalls))
	}
	wantNames := []string{"workspace", "sandbox_exec"}
	for i, want := range wantNames {
		if stored.ToolCalls[i].Name != want {
			t.Fatalf("ToolCalls[%d].Name = %q, want %q", i, stored.ToolCalls[i].Name, want)
		}
	}
	if stored.ToolCalls[0].Arguments["operation"] != "list" {
		t.Fatalf("workspace operation = %q, want list", stored.ToolCalls[0].Arguments["operation"])
	}
	if stored.ToolCalls[1].Arguments["command"] != "pwd && ls -la /workspace/input" {
		t.Fatalf("sandbox_exec command = %q, want pwd && ls -la /workspace/input", stored.ToolCalls[1].Arguments["command"])
	}
	if stored.ToolCalls[1].Result != "exit_code: 0\nstdout:\n/workspace/work\n" {
		t.Fatalf("sandbox_exec result = %q, want command output", stored.ToolCalls[1].Result)
	}
	if !strings.Contains(stored.AgentSystemPrompt, "workspace: Lists workspace metadata") {
		t.Fatalf("AgentSystemPrompt = %q, want workspace", stored.AgentSystemPrompt)
	}
	if !strings.Contains(stored.AgentSystemPrompt, "sandbox_exec: Runs shell commands") {
		t.Fatalf("AgentSystemPrompt = %q, want sandbox_exec", stored.AgentSystemPrompt)
	}
	assertAgentTurnStatuses(t, stored.AgentTurns, []string{
		run.AgentTurnStatusToolCall,
		run.AgentTurnStatusToolCall,
		run.AgentTurnStatusFinal,
	})
}

func TestWorkerPassesPromptOnlyWorkspaceContextToAgentTools(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	queue := &fakeRunQueue{}
	publisher := &recordingPublisher{}
	workspace := &recordingWorkspaceProvider{path: "/tmp/workspace"}
	now := time.Unix(100, 0).UTC()

	queueRun(ctx, t, repo, queue, now)

	tools := &recordingWorkflowToolRunner{}
	worker := newWorkerWithWorkspace(
		queue,
		repo,
		publisher,
		now,
		applicationagent.NewRunner(&toolCallingModelClient{}, tools),
		workspace,
	)
	if err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("process one: %v", err)
	}

	if !workspace.prepareCalled {
		t.Fatal("workspace prepare was not called")
	}
	if !workspace.cleanupCalled {
		t.Fatal("workspace cleanup was not called")
	}
	if len(tools.listRequests) == 0 {
		t.Fatal("tool list request was not recorded")
	}
	if tools.listRequests[0].Context.Workspace != workspace.workspace(false) {
		t.Fatalf("list workspace = %#v, want %#v", tools.listRequests[0].Context.Workspace, workspace.workspace(false))
	}
	if tools.listRequests[0].Context.Workspace.HasFiles {
		t.Fatal("list workspace HasFiles = true, want false")
	}
	if tools.listRequests[0].Context.Sandbox.ID != "" {
		t.Fatalf("list sandbox id = %q, want empty", tools.listRequests[0].Context.Sandbox.ID)
	}
	if len(tools.calls) != 1 {
		t.Fatalf("len(tool calls) = %d, want 1", len(tools.calls))
	}
	if tools.calls[0].Context.Workspace != workspace.workspace(false) {
		t.Fatalf("tool workspace = %#v, want %#v", tools.calls[0].Context.Workspace, workspace.workspace(false))
	}
	if tools.calls[0].Context.Workspace.HasFiles {
		t.Fatal("tool workspace HasFiles = true, want false")
	}
	if tools.calls[0].Context.Sandbox.ID != "" {
		t.Fatalf("tool sandbox id = %q, want empty", tools.calls[0].Context.Sandbox.ID)
	}
}

func TestWorkerPreparesWorkspaceWithoutAttachments(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	queue := &fakeRunQueue{}
	publisher := &recordingPublisher{}
	workspace := &recordingWorkspaceProvider{path: "/tmp/workspace"}
	now := time.Unix(100, 0).UTC()

	queueRun(ctx, t, repo, queue, now)

	worker := newWorkerWithWorkspace(
		queue,
		repo,
		publisher,
		now,
		applicationagent.NewRunner(fakeModelClient{}, fakeToolRunner{}),
		workspace,
	)
	if err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("process one: %v", err)
	}
	if !workspace.prepareCalled {
		t.Fatal("workspace prepare was not called")
	}
	if len(workspace.attachments) != 0 {
		t.Fatalf("workspace attachments = %#v, want none", workspace.attachments)
	}
	if !workspace.cleanupCalled {
		t.Fatal("workspace cleanup was not called")
	}
}

func TestWorkerPreparesCommandSandboxForPromptOnlyRun(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	queue := &fakeRunQueue{}
	publisher := &recordingPublisher{}
	workspace := &recordingWorkspaceProvider{path: "/tmp/workspace"}
	sandbox := &recordingSandboxProvider{supportsCommands: true}
	now := time.Unix(100, 0).UTC()

	queueRun(ctx, t, repo, queue, now)

	worker := newWorkerWithWorkspaceAndSandbox(
		queue,
		repo,
		publisher,
		now,
		applicationagent.NewRunner(fakeModelClient{}, fakeToolRunner{}),
		workspace,
		sandbox,
	)
	if err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("process one: %v", err)
	}
	if !sandbox.prepareCalled {
		t.Fatal("sandbox prepare was not called")
	}
	if sandbox.workspace != workspace.workspace(false) {
		t.Fatalf("sandbox workspace = %#v, want %#v", sandbox.workspace, workspace.workspace(false))
	}
	if !sandbox.cleanupCalled {
		t.Fatal("sandbox cleanup was not called")
	}
}

func TestWorkerDoesNotPrepareNoopSandboxForPromptOnlyRun(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	queue := &fakeRunQueue{}
	publisher := &recordingPublisher{}
	workspace := &recordingWorkspaceProvider{path: "/tmp/workspace"}
	sandbox := &recordingSandboxProvider{supportsCommands: false}
	now := time.Unix(100, 0).UTC()

	queueRun(ctx, t, repo, queue, now)

	worker := newWorkerWithWorkspaceAndSandbox(
		queue,
		repo,
		publisher,
		now,
		applicationagent.NewRunner(fakeModelClient{}, fakeToolRunner{}),
		workspace,
		sandbox,
	)
	if err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("process one: %v", err)
	}
	if !workspace.prepareCalled {
		t.Fatal("workspace prepare was not called")
	}
	if sandbox.prepareCalled {
		t.Fatal("sandbox prepare was called")
	}
	if sandbox.cleanupCalled {
		t.Fatal("sandbox cleanup was called")
	}
}

func TestWorkerPreparesWorkspaceForAttachmentsAndPassesPathToAgentTools(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	queue := &fakeRunQueue{}
	publisher := &recordingPublisher{}
	workspace := &recordingWorkspaceProvider{path: "/tmp/workspace"}
	now := time.Unix(100, 0).UTC()

	item := queueRunWithTask(ctx, t, repo, queue, run.TaskSpec{
		Prompt: "do work",
		Attachments: []run.TaskAttachment{
			{
				Filename:  "README.md",
				MediaType: "text/markdown",
				Content:   []byte("# Demo\n"),
				SizeBytes: 7,
			},
		},
	}, now)

	tools := &recordingWorkflowToolRunner{}
	worker := newWorkerWithWorkspace(
		queue,
		repo,
		publisher,
		now,
		applicationagent.NewRunner(&toolCallingModelClient{}, tools),
		workspace,
	)
	if err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("process one: %v", err)
	}

	if !workspace.prepareCalled {
		t.Fatal("workspace prepare was not called")
	}
	if workspace.runID != item.ID {
		t.Fatalf("workspace run ID = %s, want %s", workspace.runID, item.ID)
	}
	if len(workspace.attachments) != 1 || workspace.attachments[0].Filename != "README.md" {
		t.Fatalf("workspace attachments = %#v, want README.md", workspace.attachments)
	}
	if !workspace.cleanupCalled {
		t.Fatal("workspace cleanup was not called")
	}
	if workspace.cleanupContextErr != nil {
		t.Fatalf("workspace cleanup context error = %v, want nil", workspace.cleanupContextErr)
	}
	if len(tools.listRequests) == 0 {
		t.Fatal("tool list request was not recorded")
	}
	if tools.listRequests[0].Context.Workspace != workspace.workspace(true) {
		t.Fatalf("list workspace = %#v, want %#v", tools.listRequests[0].Context.Workspace, workspace.workspace(true))
	}
	if !tools.listRequests[0].Context.Workspace.HasFiles {
		t.Fatal("list workspace HasFiles = false, want true")
	}
	if len(tools.calls) != 1 {
		t.Fatalf("len(tool calls) = %d, want 1", len(tools.calls))
	}
	if tools.calls[0].Context.Workspace != workspace.workspace(true) {
		t.Fatalf("tool workspace = %#v, want %#v", tools.calls[0].Context.Workspace, workspace.workspace(true))
	}
	if !tools.calls[0].Context.Workspace.HasFiles {
		t.Fatal("tool workspace HasFiles = false, want true")
	}
	if tools.calls[0].Context.Sandbox.ID != "" {
		t.Fatalf("tool sandbox id = %q, want empty", tools.calls[0].Context.Sandbox.ID)
	}
}

func TestWorkerDoesNotPrepareNoopSandboxForAttachments(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	queue := &fakeRunQueue{}
	publisher := &recordingPublisher{}
	workspace := &recordingWorkspaceProvider{path: "/tmp/workspace"}
	sandbox := &recordingSandboxProvider{supportsCommands: false}
	now := time.Unix(100, 0).UTC()

	queueRunWithTask(ctx, t, repo, queue, textAttachmentTask(), now)

	tools := &recordingWorkflowToolRunner{}
	worker := newWorkerWithWorkspaceAndSandbox(
		queue,
		repo,
		publisher,
		now,
		applicationagent.NewRunner(&toolCallingModelClient{}, tools),
		workspace,
		sandbox,
	)
	if err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("process one: %v", err)
	}
	if sandbox.prepareCalled {
		t.Fatal("sandbox prepare was called")
	}
	if sandbox.cleanupCalled {
		t.Fatal("sandbox cleanup was called")
	}
	if tools.calls[0].Context.Sandbox.ID != "" {
		t.Fatalf("tool sandbox id = %q, want empty", tools.calls[0].Context.Sandbox.ID)
	}
}

func TestWorkerPreparesCommandSandboxForAttachments(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	queue := &fakeRunQueue{}
	publisher := &recordingPublisher{}
	workspace := &recordingWorkspaceProvider{path: "/tmp/workspace"}
	sandbox := &recordingSandboxProvider{
		supportsCommands: true,
		sandbox:          outbound.Sandbox{ID: "sandbox_test", SupportsCommands: true},
	}
	now := time.Unix(100, 0).UTC()

	item := queueRunWithTask(ctx, t, repo, queue, textAttachmentTask(), now)

	tools := &recordingWorkflowToolRunner{}
	worker := newWorkerWithWorkspaceAndSandbox(
		queue,
		repo,
		publisher,
		now,
		applicationagent.NewRunner(&toolCallingModelClient{}, tools),
		workspace,
		sandbox,
	)
	if err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("process one: %v", err)
	}

	if !sandbox.prepareCalled {
		t.Fatal("sandbox prepare was not called")
	}
	if sandbox.runID != item.ID {
		t.Fatalf("sandbox run ID = %s, want %s", sandbox.runID, item.ID)
	}
	if sandbox.workspace != workspace.workspace(true) {
		t.Fatalf("sandbox workspace = %#v, want %#v", sandbox.workspace, workspace.workspace(true))
	}
	if !sandbox.cleanupCalled {
		t.Fatal("sandbox cleanup was not called")
	}
	if sandbox.cleanupContextErr != nil {
		t.Fatalf("sandbox cleanup context error = %v, want nil", sandbox.cleanupContextErr)
	}
	if tools.listRequests[0].Context.Sandbox.ID != "sandbox_test" {
		t.Fatalf("list sandbox id = %q, want sandbox_test", tools.listRequests[0].Context.Sandbox.ID)
	}
	if !tools.listRequests[0].Context.Sandbox.SupportsCommands {
		t.Fatal("list sandbox supports commands = false, want true")
	}
	if tools.calls[0].Context.Sandbox.ID != "sandbox_test" {
		t.Fatalf("tool sandbox id = %q, want sandbox_test", tools.calls[0].Context.Sandbox.ID)
	}
	if !tools.calls[0].Context.Sandbox.SupportsCommands {
		t.Fatal("tool sandbox supports commands = false, want true")
	}
}

func TestWorkerProcessOneEmptyQueue(t *testing.T) {
	ctx := context.Background()
	worker := newWorker(&fakeRunQueue{}, newFakeRunRepository(), &recordingPublisher{}, time.Unix(100, 0).UTC())

	if err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("process empty queue: %v", err)
	}
}

func TestWorkerProcessOneKeepsCompletedRunWhenCompletedEventFails(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	queue := &fakeRunQueue{}
	publisher := &failingPublisher{failOn: outbound.EventRunCompleted}
	now := time.Unix(100, 0).UTC()

	item := queueRun(ctx, t, repo, queue, now)

	worker := newWorker(queue, repo, publisher, now)
	err := worker.ProcessOne(ctx)
	if !errors.Is(err, errPublishFailed) {
		t.Fatalf("process one error = %v, want %v", err, errPublishFailed)
	}

	stored := findRun(ctx, t, repo, item.ID)
	if stored.Status != run.StatusCompleted {
		t.Fatalf("stored status = %s, want %s", stored.Status, run.StatusCompleted)
	}
	assertSteps(t, stored.Steps, []wantStep{
		{
			name:    "prepare",
			status:  run.StatusCompleted,
			message: "Prepared policy, secrets, and source context",
			ended:   true,
		},
		{
			name:    "agent",
			status:  run.StatusCompleted,
			message: "Agent generated result summary",
			ended:   true,
		},
	})

	assertEventNotPublished(t, publisher.events, outbound.EventRunFailed)
}

func TestWorkerProcessOneStoresSanitizedFailureReasonWhenExecutionFails(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	queue := &fakeRunQueue{}
	publisher := &recordingPublisher{}
	now := time.Unix(100, 0).UTC()

	item := queueRun(ctx, t, repo, queue, now)

	worker := newWorkerWithModel(
		queue,
		repo,
		publisher,
		now,
		failingModelClient{},
	)
	if err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("process one: %v", err)
	}

	stored := findRun(ctx, t, repo, item.ID)
	if stored.Status != run.StatusFailed {
		t.Fatalf("stored status = %s, want %s", stored.Status, run.StatusFailed)
	}
	if stored.FailureReason != "run failed" {
		t.Fatalf("stored failure reason = %q, want %q", stored.FailureReason, "run failed")
	}
	if stored.FailureReason == errModelGenerationFailed.Error() {
		t.Fatalf("stored failure reason exposes raw error: %q", stored.FailureReason)
	}
	if stored.FailureCode != run.FailureCodeModelGenerateFailed {
		t.Fatalf("stored failure code = %q, want %q", stored.FailureCode, run.FailureCodeModelGenerateFailed)
	}
	assertAgentTurnStatuses(t, stored.AgentTurns, []string{run.AgentTurnStatusModelError})
	if stored.FailureMessage != "model generation failed" {
		t.Fatalf("stored failure message = %q, want model generation failed", stored.FailureMessage)
	}
	if stored.FailureMessage == errModelGenerationFailed.Error() {
		t.Fatalf("stored failure message exposes raw error: %q", stored.FailureMessage)
	}
	if stored.AgentSystemPrompt == "" {
		t.Fatal("stored failed-run agent system prompt is empty")
	}
	if stored.ResultSummary != "" {
		t.Fatalf("stored result summary = %q, want empty", stored.ResultSummary)
	}
	assertSteps(t, stored.Steps, []wantStep{
		{
			name:    "prepare",
			status:  run.StatusCompleted,
			message: "Prepared policy, secrets, and source context",
			ended:   true,
		},
		{
			name:    "agent",
			status:  run.StatusFailed,
			message: "Agent execution failed",
			ended:   true,
		},
	})

	assertEventNotPublished(t, publisher.events, outbound.EventRunCompleted)
	assertEventPublished(t, publisher.events, outbound.EventRunFailed)
}

func TestWorkerProcessOneStoresPartialToolCallsWhenModelFailsAfterToolCall(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	queue := &fakeRunQueue{}
	publisher := &recordingPublisher{}
	now := time.Unix(100, 0).UTC()

	item := queueRun(ctx, t, repo, queue, now)

	worker := newWorkerWithModel(
		queue,
		repo,
		publisher,
		now,
		&modelFailingAfterToolClient{},
	)
	if err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("process one: %v", err)
	}

	stored := findRun(ctx, t, repo, item.ID)
	if stored.Status != run.StatusFailed {
		t.Fatalf("stored status = %s, want %s", stored.Status, run.StatusFailed)
	}
	if stored.FailureCode != run.FailureCodeModelGenerateFailed {
		t.Fatalf("stored failure code = %q, want %q", stored.FailureCode, run.FailureCodeModelGenerateFailed)
	}
	if len(stored.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(stored.ToolCalls))
	}
	if stored.ToolCalls[0].Name != "echo" {
		t.Fatalf("tool call name = %q, want echo", stored.ToolCalls[0].Name)
	}
	if stored.ToolCalls[0].IsError {
		t.Fatal("tool call IsError = true, want false")
	}
	assertAgentTurnStatuses(t, stored.AgentTurns, []string{
		run.AgentTurnStatusToolCall,
		run.AgentTurnStatusModelError,
	})
}

func TestWorkerProcessOneStoresToolExecutionFailureDiagnosticsAndToolCall(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	queue := &fakeRunQueue{}
	publisher := &recordingPublisher{}
	now := time.Unix(100, 0).UTC()

	item := queueRun(ctx, t, repo, queue, now)

	tools := &recordingWorkflowToolRunner{runErr: errToolExecutionFailed}
	worker := newWorkerWithAgent(
		queue,
		repo,
		publisher,
		now,
		applicationagent.NewRunner(&toolCallingModelClient{}, tools),
	)
	if err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("process one: %v", err)
	}

	stored := findRun(ctx, t, repo, item.ID)
	if stored.Status != run.StatusFailed {
		t.Fatalf("stored status = %s, want %s", stored.Status, run.StatusFailed)
	}
	if stored.FailureCode != run.FailureCodeToolExecutionFailed {
		t.Fatalf("stored failure code = %q, want %q", stored.FailureCode, run.FailureCodeToolExecutionFailed)
	}
	if stored.FailureMessage != "tool execution failed" {
		t.Fatalf("stored failure message = %q, want tool execution failed", stored.FailureMessage)
	}
	if len(stored.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(stored.ToolCalls))
	}
	if !stored.ToolCalls[0].IsError {
		t.Fatal("tool call IsError = false, want true")
	}
	if stored.ToolCalls[0].Result != "tool execution failed" {
		t.Fatalf("tool call result = %q, want tool execution failed", stored.ToolCalls[0].Result)
	}
	assertAgentTurnStatuses(t, stored.AgentTurns, []string{run.AgentTurnStatusToolCall})
}

func TestWorkerProcessOneStoresMaxTurnsFailureCode(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	queue := &fakeRunQueue{}
	publisher := &recordingPublisher{}
	now := time.Unix(100, 0).UTC()

	item := queueRun(ctx, t, repo, queue, now)

	worker := newWorkerWithAgent(
		queue,
		repo,
		publisher,
		now,
		applicationagent.NewRunner(&toolCallingModelClient{}, fakeToolRunner{}, applicationagent.WithMaxTurns(1)),
	)
	if err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("process one: %v", err)
	}

	stored := findRun(ctx, t, repo, item.ID)
	if stored.Status != run.StatusFailed {
		t.Fatalf("stored status = %s, want %s", stored.Status, run.StatusFailed)
	}
	if stored.FailureCode != run.FailureCodeAgentMaxTurns {
		t.Fatalf("stored failure code = %q, want %q", stored.FailureCode, run.FailureCodeAgentMaxTurns)
	}
	if stored.FailureMessage != "agent reached max turns" {
		t.Fatalf("stored failure message = %q, want agent reached max turns", stored.FailureMessage)
	}
	if stored.AgentSystemPrompt == "" {
		t.Fatal("stored max-turns agent system prompt is empty")
	}
	if len(stored.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(stored.ToolCalls))
	}
	assertAgentTurnStatuses(t, stored.AgentTurns, []string{
		run.AgentTurnStatusToolCall,
		run.AgentTurnStatusMaxTurns,
	})
}

func TestWorkerProcessOneStoresProtocolCorrectionAgentTurns(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	queue := &fakeRunQueue{}
	publisher := &recordingPublisher{}
	now := time.Unix(100, 0).UTC()

	item := queueRun(ctx, t, repo, queue, now)

	worker := newWorkerWithModel(
		queue,
		repo,
		publisher,
		now,
		&protocolCorrectionModelClient{},
	)
	if err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("process one: %v", err)
	}

	stored := findRun(ctx, t, repo, item.ID)
	if stored.Status != run.StatusCompleted {
		t.Fatalf("stored status = %s, want %s", stored.Status, run.StatusCompleted)
	}
	assertAgentTurnStatuses(t, stored.AgentTurns, []string{
		run.AgentTurnStatusProtocolError,
		run.AgentTurnStatusFinal,
	})
	if stored.AgentTurns[0].ResponsePreview != `{"type":"tool_result","result":"hello"}` {
		t.Fatalf("protocol preview = %q, want invalid response", stored.AgentTurns[0].ResponsePreview)
	}
	if len(stored.Steps) != 2 {
		t.Fatalf("len(Steps) = %d, want coarse prepare and agent steps", len(stored.Steps))
	}
}

func TestWorkerProcessOneRecordsFailedPrepareStep(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	queue := &fakeRunQueue{}
	publisher := &recordingPublisher{}
	now := time.Unix(100, 0).UTC()

	item := queueRun(ctx, t, repo, queue, now)

	worker := NewWorker(
		WorkerDependencies{
			Queue:      queue,
			Repo:       repo,
			StateStore: repo,
			Events:     publisher,
			Agent:      applicationagent.NewRunner(fakeModelClient{}, fakeToolRunner{}),
			Policy:     failingPolicyDecision{},
			Secrets:    fakeSecretBroker{},
		},
		WithClock(func() time.Time { return now }),
	)
	if err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("process one: %v", err)
	}

	stored := findRun(ctx, t, repo, item.ID)
	if stored.Status != run.StatusFailed {
		t.Fatalf("stored status = %s, want %s", stored.Status, run.StatusFailed)
	}
	if stored.FailureReason != "run failed" {
		t.Fatalf("stored failure reason = %q, want run failed", stored.FailureReason)
	}
	if stored.FailureCode != run.FailureCodePreparationFailed {
		t.Fatalf("stored failure code = %q, want %q", stored.FailureCode, run.FailureCodePreparationFailed)
	}
	if stored.FailureMessage != "preparation failed" {
		t.Fatalf("stored failure message = %q, want preparation failed", stored.FailureMessage)
	}
	assertSteps(t, stored.Steps, []wantStep{
		{
			name:    "prepare",
			status:  run.StatusFailed,
			message: "Preparation failed",
			ended:   true,
		},
	})

	assertEventPublished(t, publisher.events, outbound.EventRunPreparing)
	assertEventNotPublished(t, publisher.events, outbound.EventRunStarted)
	assertEventPublished(t, publisher.events, outbound.EventRunFailed)
}

func TestWorkerProcessOneDoesNotOverwriteCancellationDuringExecution(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	queue := &fakeRunQueue{}
	publisher := &recordingPublisher{}
	now := time.Unix(100, 0).UTC()

	item := queueRun(ctx, t, repo, queue, now)

	worker := newWorkerWithModel(
		queue,
		repo,
		publisher,
		now,
		cancellingModelClient{
			repo: repo,
			id:   item.ID,
			now:  now.Add(time.Second),
		},
	)
	if err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("process one: %v", err)
	}

	stored := findRun(ctx, t, repo, item.ID)
	if stored.Status != run.StatusCancelled {
		t.Fatalf("stored status = %s, want %s", stored.Status, run.StatusCancelled)
	}
	assertSteps(t, stored.Steps, []wantStep{
		{
			name:    "prepare",
			status:  run.StatusCompleted,
			message: "Prepared policy, secrets, and source context",
			ended:   true,
		},
		{
			name:    "agent",
			status:  run.StatusCancelled,
			message: "Run cancelled",
			ended:   true,
		},
	})

	assertEventNotPublished(t, publisher.events, outbound.EventRunCompleted)
	assertEventNotPublished(t, publisher.events, outbound.EventRunFailed)
}

func TestWorkerProcessOneDoesNotLeavePrepareStepRunningWhenCancelledDuringPreparation(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	queue := &fakeRunQueue{}
	publisher := &recordingPublisher{}
	now := time.Unix(100, 0).UTC()

	item := queueRun(ctx, t, repo, queue, now)

	worker := NewWorker(
		WorkerDependencies{
			Queue:      queue,
			Repo:       repo,
			StateStore: repo,
			Events:     publisher,
			Agent:      applicationagent.NewRunner(fakeModelClient{}, fakeToolRunner{}),
			Policy: cancellingPolicyDecision{
				repo: repo,
				id:   item.ID,
				now:  now.Add(time.Second),
			},
			Secrets: fakeSecretBroker{},
		},
		WithClock(func() time.Time { return now }),
	)
	if err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("process one: %v", err)
	}

	stored := findRun(ctx, t, repo, item.ID)
	if stored.Status != run.StatusCancelled {
		t.Fatalf("stored status = %s, want %s", stored.Status, run.StatusCancelled)
	}
	assertSteps(t, stored.Steps, []wantStep{
		{
			name:    "prepare",
			status:  run.StatusCancelled,
			message: "Run cancelled",
			ended:   true,
		},
	})

	assertEventNotPublished(t, publisher.events, outbound.EventRunStarted)
	assertEventNotPublished(t, publisher.events, outbound.EventRunCompleted)
	assertEventNotPublished(t, publisher.events, outbound.EventRunFailed)
}

func TestWorkerProcessOneDoesNotOverwriteCancellationWhenExecutionFails(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	queue := &fakeRunQueue{}
	publisher := &recordingPublisher{}
	now := time.Unix(100, 0).UTC()

	item := queueRun(ctx, t, repo, queue, now)

	worker := newWorkerWithModel(
		queue,
		repo,
		publisher,
		now,
		cancellingFailingModelClient{
			repo: repo,
			id:   item.ID,
			now:  now.Add(time.Second),
		},
	)
	if err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("process one: %v", err)
	}

	stored := findRun(ctx, t, repo, item.ID)
	if stored.Status != run.StatusCancelled {
		t.Fatalf("stored status = %s, want %s", stored.Status, run.StatusCancelled)
	}
	assertSteps(t, stored.Steps, []wantStep{
		{
			name:    "prepare",
			status:  run.StatusCompleted,
			message: "Prepared policy, secrets, and source context",
			ended:   true,
		},
		{
			name:    "agent",
			status:  run.StatusCancelled,
			message: "Run cancelled",
			ended:   true,
		},
	})

	assertEventNotPublished(t, publisher.events, outbound.EventRunCompleted)
	assertEventNotPublished(t, publisher.events, outbound.EventRunFailed)
}

func queueRun(
	ctx context.Context,
	t *testing.T,
	repo *fakeRunRepository,
	queue outbound.RunQueue,
	now time.Time,
) *run.Run {
	t.Helper()

	return queueRunWithTask(ctx, t, repo, queue, run.TaskSpec{Prompt: "do work"}, now)
}

func queueRunWithTask(
	ctx context.Context,
	t *testing.T,
	repo *fakeRunRepository,
	queue outbound.RunQueue,
	task run.TaskSpec,
	now time.Time,
) *run.Run {
	t.Helper()

	item, err := run.New("run_test", task, now)
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	if err := repo.Save(ctx, item); err != nil {
		t.Fatalf("save run: %v", err)
	}
	if err := queue.Enqueue(ctx, item.ID); err != nil {
		t.Fatalf("enqueue run: %v", err)
	}

	return item
}

func textAttachmentTask() run.TaskSpec {
	return run.TaskSpec{
		Prompt: "do work",
		Attachments: []run.TaskAttachment{
			{
				Filename:  "README.md",
				MediaType: "text/markdown",
				Content:   []byte("# Demo\n"),
				SizeBytes: 7,
			},
		},
	}
}

func findRun(ctx context.Context, t *testing.T, repo *fakeRunRepository, id run.RunID) *run.Run {
	t.Helper()

	item, err := repo.FindByID(ctx, id)
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	return item
}

func newWorker(
	queue outbound.RunQueue,
	repo *fakeRunRepository,
	publisher outbound.EventPublisher,
	now time.Time,
) *Worker {
	return newWorkerWithModel(
		queue,
		repo,
		publisher,
		now,
		fakeModelClient{},
	)
}

func newWorkerWithModel(
	queue outbound.RunQueue,
	repo *fakeRunRepository,
	publisher outbound.EventPublisher,
	now time.Time,
	model outbound.ModelClient,
) *Worker {
	return newWorkerWithAgent(
		queue,
		repo,
		publisher,
		now,
		applicationagent.NewRunner(model, fakeToolRunner{}),
	)
}

func newWorkerWithAgent(
	queue outbound.RunQueue,
	repo *fakeRunRepository,
	publisher outbound.EventPublisher,
	now time.Time,
	agentRunner *applicationagent.Runner,
) *Worker {
	return NewWorker(
		WorkerDependencies{
			Queue:      queue,
			Repo:       repo,
			StateStore: repo,
			Events:     publisher,
			Agent:      agentRunner,
			Workspace:  &recordingWorkspaceProvider{path: "/tmp/workspace"},
			Policy:     fakePolicyDecision{},
			Secrets:    fakeSecretBroker{},
		},
		WithClock(func() time.Time { return now }),
	)
}

func newWorkerWithWorkspace(
	queue outbound.RunQueue,
	repo *fakeRunRepository,
	publisher outbound.EventPublisher,
	now time.Time,
	agentRunner *applicationagent.Runner,
	workspace outbound.WorkspaceProvider,
) *Worker {
	return NewWorker(
		WorkerDependencies{
			Queue:      queue,
			Repo:       repo,
			StateStore: repo,
			Events:     publisher,
			Agent:      agentRunner,
			Workspace:  workspace,
			Policy:     fakePolicyDecision{},
			Secrets:    fakeSecretBroker{},
		},
		WithClock(func() time.Time { return now }),
	)
}

func newWorkerWithWorkspaceAndSandbox(
	queue outbound.RunQueue,
	repo *fakeRunRepository,
	publisher outbound.EventPublisher,
	now time.Time,
	agentRunner *applicationagent.Runner,
	workspace outbound.WorkspaceProvider,
	sandbox outbound.SandboxProvider,
) *Worker {
	return NewWorker(
		WorkerDependencies{
			Queue:      queue,
			Repo:       repo,
			StateStore: repo,
			Events:     publisher,
			Agent:      agentRunner,
			Workspace:  workspace,
			Sandbox:    sandbox,
			Policy:     fakePolicyDecision{},
			Secrets:    fakeSecretBroker{},
		},
		WithClock(func() time.Time { return now }),
	)
}

type fakeRunRepository struct {
	items map[run.RunID]*run.Run
}

func newFakeRunRepository() *fakeRunRepository {
	return &fakeRunRepository{
		items: make(map[run.RunID]*run.Run),
	}
}

func (r *fakeRunRepository) Save(_ context.Context, item *run.Run) error {
	r.items[item.ID] = item.Clone()

	return nil
}

func (r *fakeRunRepository) SaveIfStatus(_ context.Context, item *run.Run, expected run.Status) (bool, error) {
	stored, ok := r.items[item.ID]
	if !ok {
		return false, run.ErrRunNotFound
	}
	if stored.Status != expected {
		return false, nil
	}

	r.items[item.ID] = item.Clone()

	return true, nil
}

func (r *fakeRunRepository) FindByID(_ context.Context, id run.RunID) (*run.Run, error) {
	item, ok := r.items[id]
	if !ok {
		return nil, run.ErrRunNotFound
	}

	return item.Clone(), nil
}

type fakeRunQueue struct {
	ids []run.RunID
}

func (q *fakeRunQueue) Enqueue(_ context.Context, id run.RunID) error {
	q.ids = append(q.ids, id)

	return nil
}

func (q *fakeRunQueue) Dequeue(context.Context) (run.RunID, error) {
	if len(q.ids) == 0 {
		return "", outbound.ErrRunQueueEmpty
	}

	id := q.ids[0]
	q.ids = q.ids[1:]

	return id, nil
}

type fakeModelClient struct{}

func (c fakeModelClient) Generate(context.Context, outbound.ModelRequest) (outbound.ModelResponse, error) {
	return outbound.ModelResponse{Content: "done"}, nil
}

type toolCallingModelClient struct {
	calls int
}

func (c *toolCallingModelClient) Generate(context.Context, outbound.ModelRequest) (outbound.ModelResponse, error) {
	c.calls++
	if c.calls == 1 {
		return outbound.ModelResponse{
			Content: `{"type":"tool_call","tool":"echo","arguments":{"text":"hello"}}`,
		}, nil
	}

	return outbound.ModelResponse{Content: `{"type":"final","summary":"done with tool"}`}, nil
}

type protocolCorrectionModelClient struct {
	calls int
}

func (c *protocolCorrectionModelClient) Generate(
	context.Context,
	outbound.ModelRequest,
) (outbound.ModelResponse, error) {
	c.calls++
	if c.calls == 1 {
		return outbound.ModelResponse{Content: `{"type":"tool_result","result":"hello"}`}, nil
	}

	return outbound.ModelResponse{Content: `{"type":"final","summary":"corrected"}`}, nil
}

type modelFailingAfterToolClient struct {
	calls int
}

func (c *modelFailingAfterToolClient) Generate(
	context.Context,
	outbound.ModelRequest,
) (outbound.ModelResponse, error) {
	c.calls++
	if c.calls == 1 {
		return outbound.ModelResponse{
			Content: `{"type":"tool_call","tool":"echo","arguments":{"text":"hello"}}`,
		}, nil
	}

	return outbound.ModelResponse{}, errModelGenerationFailed
}

type workspaceAndSandboxToolCallingModelClient struct {
	calls int
}

func (c *workspaceAndSandboxToolCallingModelClient) Generate(
	context.Context,
	outbound.ModelRequest,
) (outbound.ModelResponse, error) {
	c.calls++
	switch c.calls {
	case 1:
		return outbound.ModelResponse{
			Content: `{"type":"tool_call","tool":"workspace","arguments":{"operation":"list","area":"all","path":"."}}`,
		}, nil
	case 2:
		return outbound.ModelResponse{
			Content: `{"type":"tool_call","tool":"sandbox_exec","arguments":{"command":"pwd && ls -la /workspace/input"}}`,
		}, nil
	default:
		return outbound.ModelResponse{Content: `{"type":"final","summary":"done with tools"}`}, nil
	}
}

type cancellingModelClient struct {
	repo *fakeRunRepository
	id   run.RunID
	now  time.Time
}

func (c cancellingModelClient) Generate(ctx context.Context, _ outbound.ModelRequest) (outbound.ModelResponse, error) {
	item, err := c.repo.FindByID(ctx, c.id)
	if err != nil {
		return outbound.ModelResponse{}, err
	}
	if err := item.Cancel(c.now); err != nil {
		return outbound.ModelResponse{}, err
	}
	if err := c.repo.Save(ctx, item); err != nil {
		return outbound.ModelResponse{}, err
	}

	return outbound.ModelResponse{Content: "cancelled"}, nil
}

var errModelGenerationFailed = errors.New("model generation failed: provider body contained api_key=secret")

type failingModelClient struct{}

func (c failingModelClient) Generate(context.Context, outbound.ModelRequest) (outbound.ModelResponse, error) {
	return outbound.ModelResponse{}, errModelGenerationFailed
}

type cancellingFailingModelClient struct {
	repo *fakeRunRepository
	id   run.RunID
	now  time.Time
}

func (c cancellingFailingModelClient) Generate(ctx context.Context, _ outbound.ModelRequest) (outbound.ModelResponse, error) {
	item, err := c.repo.FindByID(ctx, c.id)
	if err != nil {
		return outbound.ModelResponse{}, err
	}
	if err := item.Cancel(c.now); err != nil {
		return outbound.ModelResponse{}, err
	}
	if err := c.repo.Save(ctx, item); err != nil {
		return outbound.ModelResponse{}, err
	}

	return outbound.ModelResponse{}, errModelGenerationFailed
}

type fakePolicyDecision struct{}

func (p fakePolicyDecision) Decide(context.Context, outbound.PolicyDecisionRequest) (outbound.PolicyDecision, error) {
	return outbound.PolicyDecision{Allowed: true}, nil
}

var errPolicyFailed = errors.New("policy failed: product ACL details")

type failingPolicyDecision struct{}

func (p failingPolicyDecision) Decide(context.Context, outbound.PolicyDecisionRequest) (outbound.PolicyDecision, error) {
	return outbound.PolicyDecision{}, errPolicyFailed
}

type cancellingPolicyDecision struct {
	repo *fakeRunRepository
	id   run.RunID
	now  time.Time
}

func (p cancellingPolicyDecision) Decide(ctx context.Context, _ outbound.PolicyDecisionRequest) (outbound.PolicyDecision, error) {
	item, err := p.repo.FindByID(ctx, p.id)
	if err != nil {
		return outbound.PolicyDecision{}, err
	}
	if err := item.Cancel(p.now); err != nil {
		return outbound.PolicyDecision{}, err
	}
	if err := p.repo.Save(ctx, item); err != nil {
		return outbound.PolicyDecision{}, err
	}

	return outbound.PolicyDecision{Allowed: true}, nil
}

type fakeSecretBroker struct{}

func (b fakeSecretBroker) Resolve(context.Context, outbound.SecretRequest) (outbound.SecretBundle, error) {
	return outbound.SecretBundle{Values: map[string]string{}}, nil
}

type recordingWorkspaceProvider struct {
	prepareCalled     bool
	cleanupCalled     bool
	cleanupContextErr error
	path              string
	runID             run.RunID
	attachments       []run.TaskAttachment
}

func (p *recordingWorkspaceProvider) PrepareWorkspace(
	_ context.Context,
	request outbound.WorkspacePrepareRequest,
) (outbound.Workspace, error) {
	p.prepareCalled = true
	p.runID = request.RunID
	p.attachments = append([]run.TaskAttachment(nil), request.Attachments...)

	return p.workspace(len(request.Attachments) > 0), nil
}

func (p *recordingWorkspaceProvider) CleanupWorkspace(ctx context.Context, _ outbound.Workspace) error {
	p.cleanupCalled = true
	p.cleanupContextErr = ctx.Err()

	return nil
}

func (p *recordingWorkspaceProvider) workspace(hasFiles bool) outbound.Workspace {
	return outbound.Workspace{
		RootPath:  p.path,
		InputPath: p.path + "/input",
		WorkPath:  p.path + "/work",
		HasFiles:  hasFiles,
	}
}

type recordingSandboxProvider struct {
	prepareCalled     bool
	cleanupCalled     bool
	cleanupContextErr error
	supportsCommands  bool
	sandbox           outbound.Sandbox
	runID             run.RunID
	workspace         outbound.Workspace
}

func (p *recordingSandboxProvider) Capabilities() outbound.SandboxCapabilities {
	return outbound.SandboxCapabilities{SupportsCommands: p.supportsCommands}
}

func (p *recordingSandboxProvider) Prepare(
	_ context.Context,
	request outbound.SandboxRequest,
) (outbound.Sandbox, error) {
	p.prepareCalled = true
	p.runID = request.RunID
	p.workspace = request.Workspace

	if p.sandbox.ID != "" {
		return p.sandbox, nil
	}

	return outbound.Sandbox{ID: "sandbox_test", SupportsCommands: p.supportsCommands}, nil
}

func (p *recordingSandboxProvider) Cleanup(ctx context.Context, _ outbound.Sandbox) error {
	p.cleanupCalled = true
	p.cleanupContextErr = ctx.Err()

	return nil
}

type fakeToolRunner struct{}

func (r fakeToolRunner) ListTools(context.Context, outbound.ToolListRequest) ([]outbound.ToolDefinition, error) {
	return []outbound.ToolDefinition{
		{Name: "echo", Description: "Returns text"},
		{Name: "workspace", Description: "Lists workspace metadata"},
		{Name: "sandbox_exec", Description: "Runs shell commands"},
	}, nil
}

func (r fakeToolRunner) RunTool(_ context.Context, call outbound.ToolCall) (outbound.ToolResult, error) {
	switch call.Name {
	case "workspace":
		return outbound.ToolResult{Content: "files:\n/workspace/input/README.md"}, nil
	case "sandbox_exec":
		return outbound.ToolResult{Content: "exit_code: 0\nstdout:\n/workspace/work\n"}, nil
	default:
		return outbound.ToolResult{Content: "tool result"}, nil
	}
}

type recordingWorkflowToolRunner struct {
	listRequests []outbound.ToolListRequest
	calls        []outbound.ToolCall
	runErr       error
}

func (r *recordingWorkflowToolRunner) ListTools(
	_ context.Context,
	request outbound.ToolListRequest,
) ([]outbound.ToolDefinition, error) {
	r.listRequests = append(r.listRequests, request)

	return []outbound.ToolDefinition{{Name: "echo", Description: "Returns text"}}, nil
}

func (r *recordingWorkflowToolRunner) RunTool(_ context.Context, call outbound.ToolCall) (outbound.ToolResult, error) {
	r.calls = append(r.calls, call)
	if r.runErr != nil {
		return outbound.ToolResult{}, r.runErr
	}

	return outbound.ToolResult{Content: "tool result"}, nil
}

type recordingPublisher struct {
	events []outbound.Event
}

func (p *recordingPublisher) Publish(_ context.Context, event outbound.Event) error {
	p.events = append(p.events, event)

	return nil
}

var errPublishFailed = errors.New("publish failed")
var errToolExecutionFailed = errors.New("tool failed: secret value")

type failingPublisher struct {
	failOn string
	events []outbound.Event
}

func (p *failingPublisher) Publish(_ context.Context, event outbound.Event) error {
	if event.Type == p.failOn {
		return errPublishFailed
	}

	p.events = append(p.events, event)

	return nil
}

func assertEventNotPublished(t *testing.T, events []outbound.Event, eventType string) {
	t.Helper()

	for _, event := range events {
		if event.Type == eventType {
			t.Fatalf("event %s was published unexpectedly", eventType)
		}
	}
}

func assertEventPublished(t *testing.T, events []outbound.Event, eventType string) {
	t.Helper()

	for _, event := range events {
		if event.Type == eventType {
			return
		}
	}

	t.Fatalf("event %s was not published", eventType)
}

type wantStep struct {
	name    string
	status  run.Status
	message string
	ended   bool
}

func assertSteps(t *testing.T, got []run.Step, want []wantStep) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("len(Steps) = %d, want %d; got %#v", len(got), len(want), got)
	}
	for i := range want {
		assertStep(t, i, got[i], want[i])
	}
}

func assertAgentTurnStatuses(t *testing.T, got []run.AgentTurn, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("len(AgentTurns) = %d, want %d; got %#v", len(got), len(want), got)
	}
	for i, status := range want {
		if got[i].Index != i+1 {
			t.Fatalf("AgentTurns[%d].Index = %d, want %d", i, got[i].Index, i+1)
		}
		if got[i].Status != status {
			t.Fatalf("AgentTurns[%d].Status = %q, want %q", i, got[i].Status, status)
		}
	}
}

func assertStep(t *testing.T, index int, got run.Step, want wantStep) {
	t.Helper()

	if got.Name != want.name {
		t.Fatalf("Steps[%d].Name = %q, want %q", index, got.Name, want.name)
	}
	if got.Status != want.status {
		t.Fatalf("Steps[%d].Status = %s, want %s", index, got.Status, want.status)
	}
	if got.Message != want.message {
		t.Fatalf("Steps[%d].Message = %q, want %q", index, got.Message, want.message)
	}
	if got.StartedAt.IsZero() {
		t.Fatalf("Steps[%d].StartedAt is zero", index)
	}
	if want.ended && got.EndedAt.IsZero() {
		t.Fatalf("Steps[%d].EndedAt is zero", index)
	}
	if !want.ended && !got.EndedAt.IsZero() {
		t.Fatalf("Steps[%d].EndedAt = %v, want zero", index, got.EndedAt)
	}
}
