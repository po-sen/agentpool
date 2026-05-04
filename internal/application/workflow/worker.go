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
	sandbox      outbound.SandboxProvider
	workspace    outbound.WorkspaceProvider
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
	Sandbox    outbound.SandboxProvider
	Agent      *agent.Runner
	Workspace  outbound.WorkspaceProvider
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
		sandbox:      deps.Sandbox,
		workspace:    deps.Workspace,
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

	workspace, err := w.prepareRun(ctx, item)
	if err != nil {
		if errors.Is(err, errRunStateChanged) {
			return nil
		}

		return w.failRun(ctx, item, err)
	}
	defer w.cleanupWorkspace(ctx, workspace)

	result, err := w.startRun(ctx, item, workspace.Path)
	if err != nil {
		if errors.Is(err, errRunStateChanged) {
			return nil
		}

		return w.failRun(ctx, item, err)
	}
	if err := w.completeRun(ctx, item, result); err != nil {
		if errors.Is(err, errRunStateChanged) {
			return nil
		}

		return err
	}

	return nil
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

	workspace, err := w.workspace.ResolveWorkspace(ctx, outbound.WorkspaceResolveRequest{
		RunID:  item.ID,
		Task:   item.Task,
		Source: item.Task.Workspace,
	})
	if err != nil {
		return outbound.Workspace{}, err
	}

	now = w.clock()
	expectedStatus = item.Status
	if err := item.CompleteStep(prepareStepName, prepareStepCompletedMessage, now); err != nil {
		return outbound.Workspace{}, err
	}
	if err := w.saveIfCurrentStatus(ctx, item, expectedStatus); err != nil {
		return outbound.Workspace{}, err
	}

	return workspace, nil
}

func (w *Worker) startRun(ctx context.Context, item *run.Run, workspacePath string) (agent.RunResult, error) {
	now := w.clock()
	expectedStatus := item.Status
	if err := item.StartStep(agentStepName, agentStepRunningMessage, now); err != nil {
		return agent.RunResult{}, err
	}
	if err := w.saveIfCurrentStatus(ctx, item, expectedStatus); err != nil {
		return agent.RunResult{}, err
	}

	sandbox, cleanupSandbox, err := w.prepareSandboxIfNeeded(ctx, item, workspacePath)
	if err != nil {
		return agent.RunResult{}, err
	}
	defer cleanupSandbox()

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

	// TODO: Prepare sandbox lazily when sandbox-backed tools are introduced.
	result, err := w.agent.Run(ctx, agent.RunRequest{
		RunID: item.ID,
		Task:  item.Task,
		Context: outbound.ToolContext{
			WorkspacePath: workspacePath,
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

func (w *Worker) prepareSandboxIfNeeded(
	ctx context.Context,
	item *run.Run,
	workspacePath string,
) (outbound.Sandbox, func(), error) {
	if workspacePath == "" {
		return outbound.Sandbox{}, func() {}, nil
	}
	if w.sandbox == nil {
		return outbound.Sandbox{}, func() {}, errors.New("sandbox provider is required for workspace runs")
	}

	sandbox, err := w.sandbox.Prepare(ctx, outbound.SandboxRequest{
		RunID:         item.ID,
		Task:          item.Task,
		WorkspacePath: workspacePath,
	})
	if err != nil {
		return outbound.Sandbox{}, func() {}, err
	}

	cleanup := func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sandboxCleanupTimeout)
		defer cancel()

		_ = w.sandbox.Cleanup(cleanupCtx, sandbox)
	}

	return sandbox, cleanup, nil
}

func (w *Worker) completeRun(ctx context.Context, item *run.Run, result agent.RunResult) error {
	now := w.clock()
	expectedStatus := item.Status
	if err := item.CompleteWithResult(now, result.Summary); err != nil {
		return err
	}
	if err := w.saveIfCurrentStatus(ctx, item, expectedStatus); err != nil {
		return err
	}

	return w.publish(ctx, outbound.EventRunCompleted, item.ID, now)
}

func (w *Worker) cleanupWorkspace(ctx context.Context, workspace outbound.Workspace) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), workspaceCleanupTimeout)
	defer cancel()

	_ = w.workspace.CleanupWorkspace(cleanupCtx, workspace)
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
