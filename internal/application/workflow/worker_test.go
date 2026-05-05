package workflow

import (
	"context"
	"errors"
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

func TestWorkerStoresFileAndShellToolCallHistory(t *testing.T) {
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
		applicationagent.NewRunner(&fileAndShellToolCallingModelClient{}, fakeToolRunner{}),
		workspace,
		sandbox,
	)
	if err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("process one: %v", err)
	}

	stored := findRun(ctx, t, repo, "run_test")
	if len(stored.ToolCalls) != 3 {
		t.Fatalf("len(ToolCalls) = %d, want 3", len(stored.ToolCalls))
	}
	wantNames := []string{"list_files", "read_file", "run_shell"}
	for i, want := range wantNames {
		if stored.ToolCalls[i].Name != want {
			t.Fatalf("ToolCalls[%d].Name = %q, want %q", i, stored.ToolCalls[i].Name, want)
		}
	}
	if stored.ToolCalls[1].Arguments["path"] != "README.md" {
		t.Fatalf("read_file path = %q, want README.md", stored.ToolCalls[1].Arguments["path"])
	}
	if stored.ToolCalls[2].Arguments["command"] != "pwd && ls -la" {
		t.Fatalf("run_shell command = %q, want pwd && ls -la", stored.ToolCalls[2].Arguments["command"])
	}
	if stored.ToolCalls[2].Result != "exit_code: 0\nstdout:\n/workspace\n" {
		t.Fatalf("run_shell result = %q, want shell output", stored.ToolCalls[2].Result)
	}
}

func TestWorkerPassesEmptyRuntimeContextToAgentTools(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	queue := &fakeRunQueue{}
	publisher := &recordingPublisher{}
	now := time.Unix(100, 0).UTC()

	queueRun(ctx, t, repo, queue, now)

	tools := &recordingWorkflowToolRunner{}
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

	if len(tools.listRequests) == 0 {
		t.Fatal("tool list request was not recorded")
	}
	if tools.listRequests[0].Context.WorkspacePath != "" {
		t.Fatalf("list workspace path = %q, want empty", tools.listRequests[0].Context.WorkspacePath)
	}
	if tools.listRequests[0].Context.Sandbox.ID != "" {
		t.Fatalf("list sandbox id = %q, want empty", tools.listRequests[0].Context.Sandbox.ID)
	}
	if len(tools.calls) != 1 {
		t.Fatalf("len(tool calls) = %d, want 1", len(tools.calls))
	}
	if tools.calls[0].Context.WorkspacePath != "" {
		t.Fatalf("tool workspace path = %q, want empty", tools.calls[0].Context.WorkspacePath)
	}
	if tools.calls[0].Context.Sandbox.ID != "" {
		t.Fatalf("tool sandbox id = %q, want empty", tools.calls[0].Context.Sandbox.ID)
	}
}

func TestWorkerDoesNotPrepareWorkspaceWithoutAttachments(t *testing.T) {
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
	if workspace.prepareCalled {
		t.Fatal("workspace prepare was called")
	}
	if workspace.cleanupCalled {
		t.Fatal("workspace cleanup was called")
	}
}

func TestWorkerDoesNotPrepareSandboxForPromptOnlyRun(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	queue := &fakeRunQueue{}
	publisher := &recordingPublisher{}
	sandbox := &recordingSandboxProvider{supportsCommands: true}
	now := time.Unix(100, 0).UTC()

	queueRun(ctx, t, repo, queue, now)

	worker := newWorkerWithWorkspaceAndSandbox(
		queue,
		repo,
		publisher,
		now,
		applicationagent.NewRunner(fakeModelClient{}, fakeToolRunner{}),
		&recordingWorkspaceProvider{path: "/tmp/workspace"},
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
	if tools.listRequests[0].Context.WorkspacePath != workspace.path {
		t.Fatalf("list workspace path = %q, want %q", tools.listRequests[0].Context.WorkspacePath, workspace.path)
	}
	if len(tools.calls) != 1 {
		t.Fatalf("len(tool calls) = %d, want 1", len(tools.calls))
	}
	if tools.calls[0].Context.WorkspacePath != workspace.path {
		t.Fatalf("tool workspace path = %q, want %q", tools.calls[0].Context.WorkspacePath, workspace.path)
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
	if sandbox.workspacePath != workspace.path {
		t.Fatalf("sandbox workspace path = %q, want %q", sandbox.workspacePath, workspace.path)
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
	if stored.FailureMessage != "model generation failed" {
		t.Fatalf("stored failure message = %q, want model generation failed", stored.FailureMessage)
	}
	if stored.FailureMessage == errModelGenerationFailed.Error() {
		t.Fatalf("stored failure message exposes raw error: %q", stored.FailureMessage)
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
	if len(stored.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(stored.ToolCalls))
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

type fileAndShellToolCallingModelClient struct {
	calls int
}

func (c *fileAndShellToolCallingModelClient) Generate(
	context.Context,
	outbound.ModelRequest,
) (outbound.ModelResponse, error) {
	c.calls++
	switch c.calls {
	case 1:
		return outbound.ModelResponse{Content: `{"type":"tool_call","tool":"list_files","arguments":{}}`}, nil
	case 2:
		return outbound.ModelResponse{
			Content: `{"type":"tool_call","tool":"read_file","arguments":{"path":"README.md"}}`,
		}, nil
	case 3:
		return outbound.ModelResponse{
			Content: `{"type":"tool_call","tool":"run_shell","arguments":{"command":"pwd && ls -la"}}`,
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

	return outbound.Workspace{Path: p.path}, nil
}

func (p *recordingWorkspaceProvider) CleanupWorkspace(ctx context.Context, _ outbound.Workspace) error {
	p.cleanupCalled = true
	p.cleanupContextErr = ctx.Err()

	return nil
}

type recordingSandboxProvider struct {
	prepareCalled     bool
	cleanupCalled     bool
	cleanupContextErr error
	supportsCommands  bool
	sandbox           outbound.Sandbox
	runID             run.RunID
	workspacePath     string
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
	p.workspacePath = request.WorkspacePath

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
	return []outbound.ToolDefinition{{Name: "echo", Description: "Returns text"}}, nil
}

func (r fakeToolRunner) RunTool(_ context.Context, call outbound.ToolCall) (outbound.ToolResult, error) {
	switch call.Name {
	case "list_files":
		return outbound.ToolResult{Content: "files:\nREADME.md"}, nil
	case "read_file":
		return outbound.ToolResult{Content: "# Demo\n"}, nil
	case "run_shell":
		return outbound.ToolResult{Content: "exit_code: 0\nstdout:\n/workspace\n"}, nil
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
