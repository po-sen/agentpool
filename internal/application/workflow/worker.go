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
	prepareStepName             = "prepare"
	prepareStepRunningMessage   = "Preparing policy, secrets, and source context"
	prepareStepCompletedMessage = "Prepared policy, secrets, and source context"
	prepareStepFailedMessage    = "Preparation failed"
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
		return w.failUnlessStateChanged(ctx, item, err)
	}
	defer w.cleanupWorkspace(ctx, workspace)

	sandbox, err := w.prepareSandbox(ctx, item, workspace)
	if err != nil {
		return w.failUnlessStateChanged(ctx, item, err)
	}
	defer w.cleanupSandbox(ctx, sandbox)

	result, err := w.startRun(ctx, item, workspace, sandbox)
	if err != nil {
		return w.failUnlessStateChanged(ctx, item, err)
	}
	if err := w.completeRun(ctx, item, result); err != nil {
		return ignoreRunStateChanged(err)
	}

	return nil
}

func (w *Worker) failUnlessStateChanged(ctx context.Context, item *run.Run, err error) error {
	if errors.Is(err, errRunStateChanged) {
		return nil
	}

	return w.failRun(ctx, item, err)
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
	if err := item.StartStep(prepareStepName, prepareStepRunningMessage, now); err != nil {
		return outbound.Workspace{}, err
	}
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

	workspace, err := w.prepareWorkspace(ctx, item)
	if err != nil {
		return outbound.Workspace{}, err
	}

	now = w.clock()
	expectedStatus = item.Status
	if err := item.CompleteStep(prepareStepName, prepareStepCompletedMessage, now); err != nil {
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
			WorkspacePath: workspace.Path,
			Sandbox:       sandbox,
		},
	})

	if err != nil {
		return agent.RunResult{}, err
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
	if len(item.Task.Attachments) == 0 {
		return outbound.Workspace{}, nil
	}
	if w.workspace == nil {
		return outbound.Workspace{}, errors.New("workspace provider is required")
	}

	return w.workspace.PrepareWorkspace(ctx, outbound.WorkspacePrepareRequest{
		RunID:       item.ID,
		Attachments: item.Task.Attachments,
	})
}

func (w *Worker) cleanupWorkspace(ctx context.Context, workspace outbound.Workspace) {
	if w.workspace == nil || workspace.Path == "" {
		return
	}

	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), workspaceCleanupTimeout)
	defer cancel()

	_ = w.workspace.CleanupWorkspace(cleanupCtx, workspace)
}

func (w *Worker) prepareSandbox(
	ctx context.Context,
	item *run.Run,
	workspace outbound.Workspace,
) (outbound.Sandbox, error) {
	if workspace.Path == "" || !sandboxSupportsCommands(w.sandbox) {
		return outbound.Sandbox{}, nil
	}

	return w.sandbox.Prepare(ctx, outbound.SandboxRequest{
		RunID:         item.ID,
		Task:          item.Task,
		WorkspacePath: workspace.Path,
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
) error {
	now := w.clock()
	expectedStatus := item.Status
	item.RecordToolCalls(now, toDomainToolCalls(result.ToolCalls))
	if err := item.CompleteWithResult(now, result.Summary); err != nil {
		return err
	}
	if err := w.saveIfCurrentStatus(ctx, item, expectedStatus); err != nil {
		return err
	}

	return w.publish(ctx, outbound.EventRunCompleted, item.ID, now)
}

func (w *Worker) failRun(ctx context.Context, item *run.Run, cause error) error {
	now := w.clock()
	expectedStatus := item.Status
	if name, ok := latestRunningStepName(item); ok {
		if err := item.FailStep(name, failedStepMessage(name), now); err != nil {
			return errors.Join(cause, err)
		}
	}
	if err := item.FailWithReason(now, publicFailureReason); err != nil {
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
	case prepareStepName:
		return prepareStepFailedMessage
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
