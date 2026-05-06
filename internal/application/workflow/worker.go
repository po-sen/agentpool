package workflow

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/po-sen/agentpool/internal/application/agent"
	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

const defaultPollInterval = 200 * time.Millisecond
const workspaceCleanupTimeout = 30 * time.Second
const sandboxCleanupTimeout = 30 * time.Second
const publicFailureReason = "run failed"

const (
	workspaceStepName           = "workspace"
	workspaceStepRunningMessage = "Preparing workspace"
	workspaceStepFailedMessage  = "Workspace preparation failed"
	agentStepName               = "agent"
	agentStepRunningMessage     = "Agent execution started"
	agentStepCompletedMessage   = "Agent generated result summary"
	agentStepFailedMessage      = "Agent execution failed"
)

var errRunStateChanged = errors.New("run state changed")

// Worker processes queued runs through abstract outbound ports.
type Worker struct {
	queue        outbound.RunQueue
	repo         run.Repository
	stateStore   outbound.RunStateStore
	events       outbound.EventPublisher
	agent        *agent.Runner
	workspace    outbound.WorkspaceProvider
	sandbox      outbound.SandboxProvider
	policy       outbound.PolicyDecisionPort
	secrets      outbound.SecretBroker
	clock        func() time.Time
	pollInterval time.Duration
}

// WorkerOption configures a Worker.
type WorkerOption func(*Worker)

// WorkerDependencies groups outbound ports required by the worker workflow.
type WorkerDependencies struct {
	Queue      outbound.RunQueue
	Repo       run.Repository
	StateStore outbound.RunStateStore
	Events     outbound.EventPublisher
	Agent      *agent.Runner
	Workspace  outbound.WorkspaceProvider
	Sandbox    outbound.SandboxProvider
	Policy     outbound.PolicyDecisionPort
	Secrets    outbound.SecretBroker
}

// WithClock injects a time source for tests.
func WithClock(clock func() time.Time) WorkerOption {
	return func(worker *Worker) {
		worker.clock = clock
	}
}

// WithPollInterval configures the worker poll interval.
func WithPollInterval(interval time.Duration) WorkerOption {
	return func(worker *Worker) {
		worker.pollInterval = interval
	}
}

// NewWorker wires outbound ports needed by the worker workflow.
func NewWorker(
	deps WorkerDependencies,
	options ...WorkerOption,
) *Worker {
	worker := &Worker{
		queue:        deps.Queue,
		repo:         deps.Repo,
		stateStore:   deps.StateStore,
		events:       deps.Events,
		agent:        deps.Agent,
		workspace:    deps.Workspace,
		sandbox:      deps.Sandbox,
		policy:       deps.Policy,
		secrets:      deps.Secrets,
		pollInterval: defaultPollInterval,
		clock: func() time.Time {
			return time.Now().UTC()
		},
	}

	for _, option := range options {
		option(worker)
	}

	return worker
}

// Run processes queued runs until the context is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		if err := w.ProcessOne(ctx); err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}

			return err
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// ProcessOne processes one queued run. Empty queues are treated as no work.
func (w *Worker) ProcessOne(ctx context.Context) error {
	id, err := w.queue.Dequeue(ctx)
	if errors.Is(err, outbound.ErrRunQueueEmpty) {
		return nil
	}
	if err != nil {
		return err
	}

	item, err := w.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if item.Status.IsTerminal() || item.Status != run.StatusQueued {
		return nil
	}

	return w.processRun(ctx, item)
}

func (w *Worker) processRun(ctx context.Context, item *run.Run) error {
	workspace, err := w.prepareRun(ctx, item)
	if err != nil {
		return w.failUnlessStateChanged(ctx, item, agent.RunResult{}, nil, err)
	}
	defer w.cleanupWorkspace(ctx, workspace)

	sandbox, err := w.prepareSandbox(ctx, item, workspace)
	if err != nil {
		artifacts := w.collectArtifacts(ctx, workspace)

		return w.failUnlessStateChanged(ctx, item, agent.RunResult{}, artifacts, err)
	}
	defer w.cleanupSandbox(ctx, sandbox)

	result, err := w.startRun(ctx, item, workspace, sandbox)
	artifacts := w.collectArtifacts(ctx, workspace)
	if err != nil {
		return w.failUnlessStateChanged(ctx, item, result, artifacts, err)
	}
	if err := w.completeRun(ctx, item, result, artifacts); err != nil {
		return ignoreRunStateChanged(err)
	}

	return nil
}

func (w *Worker) failUnlessStateChanged(
	ctx context.Context,
	item *run.Run,
	result agent.RunResult,
	artifacts []run.Artifact,
	err error,
) error {
	if errors.Is(err, errRunStateChanged) {
		return nil
	}

	return w.failRun(ctx, item, result, artifacts, err)
}

func ignoreRunStateChanged(err error) error {
	if errors.Is(err, errRunStateChanged) {
		return nil
	}

	return err
}

func (w *Worker) prepareRun(ctx context.Context, item *run.Run) (outbound.Workspace, error) {
	now := w.clock()
	expectedStatus := item.Status
	if err := item.StartPreparing(now); err != nil {
		return outbound.Workspace{}, err
	}
	if err := w.saveIfCurrentStatus(ctx, item, expectedStatus); err != nil {
		return outbound.Workspace{}, err
	}
	if err := w.publish(ctx, outbound.EventRunPreparing, item.ID, now); err != nil {
		return outbound.Workspace{}, err
	}

	decision, err := w.policy.Decide(ctx, outbound.PolicyDecisionRequest{
		RunID: item.ID,
		Task:  item.Task,
	})
	if err != nil {
		return outbound.Workspace{}, err
	}
	if !decision.Allowed {
		if decision.Reason == "" {
			return outbound.Workspace{}, errors.New("policy denied run")
		}

		return outbound.Workspace{}, errors.New(decision.Reason)
	}

	if _, err := w.secrets.Resolve(ctx, outbound.SecretRequest{
		ProjectID: item.Task.ProjectID,
	}); err != nil {
		return outbound.Workspace{}, err
	}

	now = w.clock()
	expectedStatus = item.Status
	if err := item.StartStep(workspaceStepName, workspaceStepRunningMessage, now); err != nil {
		return outbound.Workspace{}, err
	}
	if err := w.saveIfCurrentStatus(ctx, item, expectedStatus); err != nil {
		return outbound.Workspace{}, err
	}

	workspace, err := w.prepareWorkspace(ctx, item)
	if err != nil {
		return outbound.Workspace{}, err
	}

	now = w.clock()
	expectedStatus = item.Status
	if err := item.CompleteStep(workspaceStepName, workspaceCompletedMessage(len(item.Task.Attachments)), now); err != nil {
		w.cleanupWorkspace(ctx, workspace)

		return outbound.Workspace{}, err
	}
	if err := w.saveIfCurrentStatus(ctx, item, expectedStatus); err != nil {
		w.cleanupWorkspace(ctx, workspace)

		return outbound.Workspace{}, err
	}

	return workspace, nil
}

func (w *Worker) startRun(
	ctx context.Context,
	item *run.Run,
	workspace outbound.Workspace,
	sandbox outbound.Sandbox,
) (agent.RunResult, error) {
	now := w.clock()
	expectedStatus := item.Status
	if err := item.StartStep(agentStepName, agentStepRunningMessage, now); err != nil {
		return agent.RunResult{}, err
	}
	if err := w.saveIfCurrentStatus(ctx, item, expectedStatus); err != nil {
		return agent.RunResult{}, err
	}

	now = w.clock()
	expectedStatus = item.Status
	if err := item.StartRunning(now); err != nil {
		return agent.RunResult{}, err
	}
	if err := w.saveIfCurrentStatus(ctx, item, expectedStatus); err != nil {
		return agent.RunResult{}, err
	}
	if err := w.publish(ctx, outbound.EventRunStarted, item.ID, now); err != nil {
		return agent.RunResult{}, err
	}

	result, err := w.agent.Run(ctx, agent.RunRequest{
		RunID: item.ID,
		Task:  item.Task,
		Context: outbound.ToolContext{
			Workspace: workspace,
			Sandbox:   sandbox,
		},
	})

	if err != nil {
		return result, err
	}

	now = w.clock()
	expectedStatus = item.Status
	if err := item.CompleteStep(agentStepName, agentCompletedMessage(result.ToolCallCount), now); err != nil {
		return agent.RunResult{}, err
	}
	if err := w.saveIfCurrentStatus(ctx, item, expectedStatus); err != nil {
		return agent.RunResult{}, err
	}

	return result, nil
}

func (w *Worker) prepareWorkspace(ctx context.Context, item *run.Run) (outbound.Workspace, error) {
	if w.workspace == nil {
		return outbound.Workspace{}, errors.New("workspace provider is required")
	}

	return w.workspace.PrepareWorkspace(ctx, outbound.WorkspacePrepareRequest{
		RunID:       item.ID,
		Attachments: item.Task.Attachments,
	})
}

func (w *Worker) cleanupWorkspace(ctx context.Context, workspace outbound.Workspace) {
	if w.workspace == nil || workspace.RootPath == "" {
		return
	}

	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), workspaceCleanupTimeout)
	defer cancel()

	_ = w.workspace.CleanupWorkspace(cleanupCtx, workspace)
}

func (w *Worker) collectArtifacts(ctx context.Context, workspace outbound.Workspace) []run.Artifact {
	if w.workspace == nil || workspace.WorkPath == "" {
		return nil
	}

	collectCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), workspaceCleanupTimeout)
	defer cancel()

	artifacts, err := w.workspace.CollectArtifacts(collectCtx, workspace)
	if err != nil {
		return nil
	}

	return artifacts
}

func (w *Worker) prepareSandbox(
	ctx context.Context,
	item *run.Run,
	workspace outbound.Workspace,
) (outbound.Sandbox, error) {
	if workspace.InputPath == "" || workspace.WorkPath == "" || !sandboxSupportsCommands(w.sandbox) {
		return outbound.Sandbox{}, nil
	}

	return w.sandbox.Prepare(ctx, outbound.SandboxRequest{
		RunID:     item.ID,
		Task:      item.Task,
		Workspace: workspace,
	})
}

func (w *Worker) cleanupSandbox(ctx context.Context, sandbox outbound.Sandbox) {
	if w.sandbox == nil || sandbox.ID == "" {
		return
	}

	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sandboxCleanupTimeout)
	defer cancel()

	_ = w.sandbox.Cleanup(cleanupCtx, sandbox)
}

func sandboxSupportsCommands(provider outbound.SandboxProvider) bool {
	capabilities, ok := provider.(outbound.SandboxCapabilityProvider)
	if !ok {
		return false
	}

	return capabilities.Capabilities().SupportsCommands
}

func (w *Worker) completeRun(
	ctx context.Context,
	item *run.Run,
	result agent.RunResult,
	artifacts []run.Artifact,
) error {
	now := w.clock()
	expectedStatus := item.Status
	item.RecordToolCalls(now, toDomainToolCalls(result.ToolCalls))
	item.RecordAgentTurns(now, toDomainAgentTurns(result.AgentTurns))
	item.RecordArtifacts(now, artifacts)
	if result.SystemPrompt != "" {
		item.RecordAgentSystemPrompt(now, result.SystemPrompt)
	}
	if err := item.CompleteWithResult(now, result.Summary); err != nil {
		return err
	}
	if err := w.saveIfCurrentStatus(ctx, item, expectedStatus); err != nil {
		return err
	}

	return w.publish(ctx, outbound.EventRunCompleted, item.ID, now)
}

func (w *Worker) failRun(
	ctx context.Context,
	item *run.Run,
	result agent.RunResult,
	artifacts []run.Artifact,
	cause error,
) error {
	now := w.clock()
	expectedStatus := item.Status
	item.RecordToolCalls(now, toDomainToolCalls(result.ToolCalls))
	item.RecordAgentTurns(now, toDomainAgentTurns(result.AgentTurns))
	item.RecordArtifacts(now, artifacts)
	if result.SystemPrompt != "" {
		item.RecordAgentSystemPrompt(now, result.SystemPrompt)
	}
	code, message := failureDiagnosticsFor(item, cause)
	if name, ok := latestRunningStepName(item); ok {
		if err := item.FailStep(name, failedStepMessage(name), now); err != nil {
			return errors.Join(cause, err)
		}
	}
	if err := item.FailWithDiagnostics(now, publicFailureReason, code, message); err != nil {
		return errors.Join(cause, err)
	}
	if err := w.saveIfCurrentStatus(ctx, item, expectedStatus); err != nil {
		if errors.Is(err, errRunStateChanged) {
			return nil
		}

		return errors.Join(cause, err)
	}
	if err := w.publish(ctx, outbound.EventRunFailed, item.ID, now); err != nil {
		return errors.Join(cause, err)
	}

	return nil
}

func failureDiagnosticsFor(item *run.Run, cause error) (string, string) {
	var agentErr agent.Error
	if errors.As(cause, &agentErr) {
		return domainFailureCode(agentErr.Code), agentErr.Message
	}
	if stepWasRunning(item, workspaceStepName) {
		return run.FailureCodePreparationFailed, "workspace preparation failed"
	}
	if item.Status == run.StatusPreparing {
		return run.FailureCodePreparationFailed, "preparation failed"
	}

	return run.FailureCodeUnknown, publicFailureReason
}

func domainFailureCode(code agent.ErrorCode) string {
	switch code {
	case agent.ErrorCodeModelGenerateFailed:
		return run.FailureCodeModelGenerateFailed
	case agent.ErrorCodeToolListFailed:
		return run.FailureCodeToolListFailed
	case agent.ErrorCodeToolExecutionFailed:
		return run.FailureCodeToolExecutionFailed
	case agent.ErrorCodeAgentMaxTurns:
		return run.FailureCodeAgentMaxTurns
	case agent.ErrorCodeFinalSummaryInvalid:
		return run.FailureCodeFinalSummaryInvalid
	case agent.ErrorCodeAgentProtocolError:
		return run.FailureCodeAgentProtocolError
	default:
		return run.FailureCodeUnknown
	}
}

func stepWasRunning(item *run.Run, name string) bool {
	for _, step := range item.Steps {
		if step.Name == name && step.Status == run.StatusRunning {
			return true
		}
	}

	return false
}

func (w *Worker) publish(ctx context.Context, eventType string, id run.RunID, occurredAt time.Time) error {
	return w.events.Publish(ctx, outbound.Event{
		Type:       eventType,
		RunID:      id,
		OccurredAt: occurredAt,
	})
}

func (w *Worker) saveIfCurrentStatus(ctx context.Context, item *run.Run, expected run.Status) error {
	saved, err := w.stateStore.SaveIfStatus(ctx, item, expected)
	if err != nil {
		return err
	}
	if !saved {
		return errRunStateChanged
	}

	return nil
}

func latestRunningStepName(item *run.Run) (string, bool) {
	for i := len(item.Steps) - 1; i >= 0; i-- {
		if item.Steps[i].Status == run.StatusRunning {
			return item.Steps[i].Name, true
		}
	}

	return "", false
}

func failedStepMessage(name string) string {
	switch name {
	case workspaceStepName:
		return workspaceStepFailedMessage
	case agentStepName:
		return agentStepFailedMessage
	default:
		return "Run step failed"
	}
}

func agentCompletedMessage(toolCallCount int) string {
	if toolCallCount <= 0 {
		return agentStepCompletedMessage
	}

	return fmt.Sprintf("Agent generated result summary after %d tool call(s)", toolCallCount)
}

func workspaceCompletedMessage(attachmentCount int) string {
	if attachmentCount <= 0 {
		return "Prepared empty workspace"
	}

	return fmt.Sprintf("Prepared workspace with %d uploaded file(s)", attachmentCount)
}

func toDomainToolCalls(records []agent.ToolCallRecord) []run.ToolCall {
	if len(records) == 0 {
		return nil
	}

	calls := make([]run.ToolCall, 0, len(records))
	for _, record := range records {
		calls = append(calls, run.ToolCall{
			Name:      record.Name,
			Arguments: copyToolArguments(record.Arguments),
			Result:    record.Result,
			IsError:   record.IsError,
			StartedAt: record.StartedAt,
			EndedAt:   record.EndedAt,
		})
	}

	return calls
}

func toDomainAgentTurns(records []agent.TurnRecord) []run.AgentTurn {
	if len(records) == 0 {
		return nil
	}

	turns := make([]run.AgentTurn, 0, len(records))
	for _, record := range records {
		turns = append(turns, run.AgentTurn{
			Index:             record.Index,
			Status:            record.Status,
			ActionType:        record.ActionType,
			ToolName:          record.ToolName,
			Message:           record.Message,
			RequestMessages:   toDomainAgentTurnMessages(record.RequestMessages),
			RawResponse:       record.RawResponse,
			ResponseFormat:    record.ResponseFormat,
			ProtocolErrorCode: record.ProtocolErrorCode,
			CorrectionMessage: record.CorrectionMessage,
			ResponsePreview:   record.ResponsePreview,
			StartedAt:         record.StartedAt,
			EndedAt:           record.EndedAt,
		})
	}

	return turns
}

func toDomainAgentTurnMessages(records []agent.TurnMessageRecord) []run.AgentTurnMessage {
	if len(records) == 0 {
		return nil
	}

	messages := make([]run.AgentTurnMessage, 0, len(records))
	for _, record := range records {
		messages = append(messages, run.AgentTurnMessage{
			Role:    record.Role,
			Content: record.Content,
		})
	}

	return messages
}

func copyToolArguments(arguments map[string]string) map[string]string {
	if len(arguments) == 0 {
		return nil
	}

	copied := make(map[string]string, len(arguments))
	for key, value := range arguments {
		copied[key] = value
	}

	return copied
}
