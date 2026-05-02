package workflow

import (
	"context"
	"errors"
	"time"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

const defaultPollInterval = 200 * time.Millisecond
const sandboxCleanupTimeout = 30 * time.Second

var errRunStateChanged = errors.New("run state changed")

// Worker processes queued runs through abstract outbound ports.
type Worker struct {
	queue        outbound.RunQueue
	repo         outbound.RunRepository
	events       outbound.EventPublisher
	sandbox      outbound.SandboxProvider
	agent        outbound.AgentExecutor
	git          outbound.GitProvider
	policy       outbound.PolicyDecisionPort
	secrets      outbound.SecretBroker
	clock        func() time.Time
	pollInterval time.Duration
}

// WorkerOption configures a Worker.
type WorkerOption func(*Worker)

// WorkerDependencies groups outbound ports required by the worker workflow.
type WorkerDependencies struct {
	Queue   outbound.RunQueue
	Repo    outbound.RunRepository
	Events  outbound.EventPublisher
	Sandbox outbound.SandboxProvider
	Agent   outbound.AgentExecutor
	Git     outbound.GitProvider
	Policy  outbound.PolicyDecisionPort
	Secrets outbound.SecretBroker
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
		events:       deps.Events,
		sandbox:      deps.Sandbox,
		agent:        deps.Agent,
		git:          deps.Git,
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

	if err := w.prepareRun(ctx, item); err != nil {
		if errors.Is(err, errRunStateChanged) {
			return nil
		}

		return w.failRun(ctx, item, err)
	}
	if err := w.startRun(ctx, item); err != nil {
		if errors.Is(err, errRunStateChanged) {
			return nil
		}

		return w.failRun(ctx, item, err)
	}
	if err := w.completeRun(ctx, item); err != nil {
		if errors.Is(err, errRunStateChanged) {
			return nil
		}

		return err
	}

	return nil
}

func (w *Worker) prepareRun(ctx context.Context, item *run.Run) error {
	now := w.clock()
	expectedStatus := item.Status
	if err := item.StartPreparing(now); err != nil {
		return err
	}
	saved, err := w.repo.SaveIfStatus(ctx, item, expectedStatus)
	if err != nil {
		return err
	}
	if !saved {
		return errRunStateChanged
	}
	if err := w.publish(ctx, outbound.EventRunPreparing, item.ID, now); err != nil {
		return err
	}

	decision, err := w.policy.Decide(ctx, outbound.PolicyDecisionRequest{
		RunID: item.ID,
		Task:  item.Task,
	})
	if err != nil {
		return err
	}
	if !decision.Allowed {
		if decision.Reason == "" {
			return errors.New("policy denied run")
		}

		return errors.New(decision.Reason)
	}

	if _, err := w.secrets.Resolve(ctx, outbound.SecretRequest{
		ProjectID: item.Task.ProjectID,
	}); err != nil {
		return err
	}

	if _, err := w.git.Fetch(ctx, outbound.GitFetchRequest{
		RepositoryURL: item.Task.RepositoryURL,
		Branch:        item.Task.Branch,
	}); err != nil {
		return err
	}

	return nil
}

func (w *Worker) startRun(ctx context.Context, item *run.Run) error {
	sandbox, err := w.sandbox.Prepare(ctx, outbound.SandboxRequest{
		RunID: item.ID,
		Task:  item.Task,
	})
	if err != nil {
		return err
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sandboxCleanupTimeout)
		defer cancel()

		_ = w.sandbox.Cleanup(cleanupCtx, sandbox)
	}()

	now := w.clock()
	expectedStatus := item.Status
	if err := item.StartRunning(now); err != nil {
		return err
	}
	saved, err := w.repo.SaveIfStatus(ctx, item, expectedStatus)
	if err != nil {
		return err
	}
	if !saved {
		return errRunStateChanged
	}
	if err := w.publish(ctx, outbound.EventRunStarted, item.ID, now); err != nil {
		return err
	}

	_, err = w.agent.Execute(ctx, outbound.AgentExecutionRequest{
		RunID:   item.ID,
		Task:    item.Task,
		Sandbox: sandbox,
	})

	return err
}

func (w *Worker) completeRun(ctx context.Context, item *run.Run) error {
	now := w.clock()
	expectedStatus := item.Status
	if err := item.Complete(now); err != nil {
		return err
	}
	saved, err := w.repo.SaveIfStatus(ctx, item, expectedStatus)
	if err != nil {
		return err
	}
	if !saved {
		return errRunStateChanged
	}

	return w.publish(ctx, outbound.EventRunCompleted, item.ID, now)
}

func (w *Worker) failRun(ctx context.Context, item *run.Run, cause error) error {
	now := w.clock()
	expectedStatus := item.Status
	if err := item.Fail(now); err != nil {
		return errors.Join(cause, err)
	}
	saved, err := w.repo.SaveIfStatus(ctx, item, expectedStatus)
	if err != nil {
		return errors.Join(cause, err)
	}
	if !saved {
		return nil
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
