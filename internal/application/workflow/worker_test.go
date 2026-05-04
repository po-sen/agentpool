package workflow_test

import (
	"context"
	"errors"
	"testing"
	"time"

	applicationagent "github.com/po-sen/agentpool/internal/application/agent"
	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/application/workflow"
	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestWorkerProcessOneCompletesQueuedRun(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	queue := &fakeRunQueue{}
	publisher := &recordingPublisher{}
	now := time.Unix(100, 0).UTC()

	item, err := run.New("run_test", run.TaskSpec{Prompt: "do work"}, now)
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	if err := repo.Save(ctx, item); err != nil {
		t.Fatalf("save run: %v", err)
	}
	if err := queue.Enqueue(ctx, item.ID); err != nil {
		t.Fatalf("enqueue run: %v", err)
	}

	worker := newWorker(queue, repo, publisher, now)
	if err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("process one: %v", err)
	}

	stored, err := repo.FindByID(ctx, item.ID)
	if err != nil {
		t.Fatalf("find run: %v", err)
	}
	if stored.Status != run.StatusCompleted {
		t.Fatalf("stored status = %s, want %s", stored.Status, run.StatusCompleted)
	}
	if stored.ResultSummary != "done" {
		t.Fatalf("stored result summary = %q, want %q", stored.ResultSummary, "done")
	}
	if stored.FailureReason != "" {
		t.Fatalf("stored failure reason = %q, want empty", stored.FailureReason)
	}

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

	item, err := run.New("run_test", run.TaskSpec{Prompt: "do work"}, now)
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	if err := repo.Save(ctx, item); err != nil {
		t.Fatalf("save run: %v", err)
	}
	if err := queue.Enqueue(ctx, item.ID); err != nil {
		t.Fatalf("enqueue run: %v", err)
	}

	worker := newWorker(queue, repo, publisher, now)
	err = worker.ProcessOne(ctx)
	if !errors.Is(err, errPublishFailed) {
		t.Fatalf("process one error = %v, want %v", err, errPublishFailed)
	}

	stored, err := repo.FindByID(ctx, item.ID)
	if err != nil {
		t.Fatalf("find run: %v", err)
	}
	if stored.Status != run.StatusCompleted {
		t.Fatalf("stored status = %s, want %s", stored.Status, run.StatusCompleted)
	}

	assertEventNotPublished(t, publisher.events, outbound.EventRunFailed)
}

func TestWorkerProcessOneStoresSanitizedFailureReasonWhenExecutionFails(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	queue := &fakeRunQueue{}
	publisher := &recordingPublisher{}
	now := time.Unix(100, 0).UTC()

	item, err := run.New("run_test", run.TaskSpec{Prompt: "do work"}, now)
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	if err := repo.Save(ctx, item); err != nil {
		t.Fatalf("save run: %v", err)
	}
	if err := queue.Enqueue(ctx, item.ID); err != nil {
		t.Fatalf("enqueue run: %v", err)
	}

	worker := newWorkerWithPorts(
		queue,
		repo,
		publisher,
		now,
		fakeSandboxProvider{},
		failingModelClient{},
		fakeGitProvider{},
	)

	if err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("process one: %v", err)
	}

	stored, err := repo.FindByID(ctx, item.ID)
	if err != nil {
		t.Fatalf("find run: %v", err)
	}
	if stored.Status != run.StatusFailed {
		t.Fatalf("stored status = %s, want %s", stored.Status, run.StatusFailed)
	}
	if stored.FailureReason != "run failed" {
		t.Fatalf("stored failure reason = %q, want %q", stored.FailureReason, "run failed")
	}
	if stored.FailureReason == errModelGenerationFailed.Error() {
		t.Fatalf("stored failure reason exposes raw error: %q", stored.FailureReason)
	}
	if stored.ResultSummary != "" {
		t.Fatalf("stored result summary = %q, want empty", stored.ResultSummary)
	}

	assertEventNotPublished(t, publisher.events, outbound.EventRunCompleted)
	assertEventPublished(t, publisher.events, outbound.EventRunFailed)
}

func TestWorkerProcessOneDoesNotOverwriteCancellationDuringExecution(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	queue := &fakeRunQueue{}
	publisher := &recordingPublisher{}
	now := time.Unix(100, 0).UTC()

	item, err := run.New("run_test", run.TaskSpec{Prompt: "do work"}, now)
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	if err := repo.Save(ctx, item); err != nil {
		t.Fatalf("save run: %v", err)
	}
	if err := queue.Enqueue(ctx, item.ID); err != nil {
		t.Fatalf("enqueue run: %v", err)
	}

	worker := newWorkerWithPorts(
		queue,
		repo,
		publisher,
		now,
		fakeSandboxProvider{},
		cancellingModelClient{
			repo: repo,
			id:   item.ID,
			now:  now.Add(time.Second),
		},
		fakeGitProvider{},
	)

	if err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("process one: %v", err)
	}

	stored, err := repo.FindByID(ctx, item.ID)
	if err != nil {
		t.Fatalf("find run: %v", err)
	}
	if stored.Status != run.StatusCancelled {
		t.Fatalf("stored status = %s, want %s", stored.Status, run.StatusCancelled)
	}

	assertEventNotPublished(t, publisher.events, outbound.EventRunCompleted)
	assertEventNotPublished(t, publisher.events, outbound.EventRunFailed)
}

func TestWorkerProcessOneDoesNotOverwriteCancellationWhenExecutionFails(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRunRepository()
	queue := &fakeRunQueue{}
	publisher := &recordingPublisher{}
	now := time.Unix(100, 0).UTC()

	item, err := run.New("run_test", run.TaskSpec{Prompt: "do work"}, now)
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	if err := repo.Save(ctx, item); err != nil {
		t.Fatalf("save run: %v", err)
	}
	if err := queue.Enqueue(ctx, item.ID); err != nil {
		t.Fatalf("enqueue run: %v", err)
	}

	worker := newWorkerWithPorts(
		queue,
		repo,
		publisher,
		now,
		fakeSandboxProvider{},
		cancellingFailingModelClient{
			repo: repo,
			id:   item.ID,
			now:  now.Add(time.Second),
		},
		fakeGitProvider{},
	)

	if err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("process one: %v", err)
	}

	stored, err := repo.FindByID(ctx, item.ID)
	if err != nil {
		t.Fatalf("find run: %v", err)
	}
	if stored.Status != run.StatusCancelled {
		t.Fatalf("stored status = %s, want %s", stored.Status, run.StatusCancelled)
	}

	assertEventNotPublished(t, publisher.events, outbound.EventRunCompleted)
	assertEventNotPublished(t, publisher.events, outbound.EventRunFailed)
}

func TestWorkerProcessOneCleansSandboxWithNonCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	repo := newFakeRunRepository()
	queue := &fakeRunQueue{}
	publisher := &recordingPublisher{}
	sandbox := &recordingSandboxProvider{}
	now := time.Unix(100, 0).UTC()

	item, err := run.New("run_test", run.TaskSpec{Prompt: "do work"}, now)
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	if err := repo.Save(ctx, item); err != nil {
		t.Fatalf("save run: %v", err)
	}
	if err := queue.Enqueue(ctx, item.ID); err != nil {
		t.Fatalf("enqueue run: %v", err)
	}

	worker := newWorkerWithPorts(
		queue,
		repo,
		publisher,
		now,
		sandbox,
		cancellingContextModelClient{cancel: cancel},
		fakeGitProvider{},
	)

	if err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("process one: %v", err)
	}
	if !sandbox.cleanupCalled {
		t.Fatal("sandbox cleanup was not called")
	}
	if sandbox.cleanupContextErr != nil {
		t.Fatalf("cleanup context error = %v, want nil", sandbox.cleanupContextErr)
	}
}

func newWorker(
	queue outbound.RunQueue,
	repo *fakeRunRepository,
	publisher outbound.EventPublisher,
	now time.Time,
) *workflow.Worker {
	return newWorkerWithPorts(
		queue,
		repo,
		publisher,
		now,
		fakeSandboxProvider{},
		fakeModelClient{},
		fakeGitProvider{},
	)
}

func newWorkerWithPorts(
	queue outbound.RunQueue,
	repo *fakeRunRepository,
	publisher outbound.EventPublisher,
	now time.Time,
	sandbox outbound.SandboxProvider,
	model outbound.ModelClient,
	git outbound.GitProvider,
) *workflow.Worker {
	return workflow.NewWorker(
		workflow.WorkerDependencies{
			Queue:      queue,
			Repo:       repo,
			StateStore: repo,
			Events:     publisher,
			Sandbox:    sandbox,
			Agent:      applicationagent.NewRunner(model),
			Git:        git,
			Policy:     fakePolicyDecision{},
			Secrets:    fakeSecretBroker{},
		},
		workflow.WithClock(func() time.Time { return now }),
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

type fakeSandboxProvider struct{}

func (p fakeSandboxProvider) Prepare(context.Context, outbound.SandboxRequest) (outbound.Sandbox, error) {
	return outbound.Sandbox{ID: "sandbox_test"}, nil
}

func (p fakeSandboxProvider) Cleanup(context.Context, outbound.Sandbox) error {
	return nil
}

type recordingSandboxProvider struct {
	cleanupCalled     bool
	cleanupContextErr error
}

func (p *recordingSandboxProvider) Prepare(context.Context, outbound.SandboxRequest) (outbound.Sandbox, error) {
	return outbound.Sandbox{ID: "sandbox_test"}, nil
}

func (p *recordingSandboxProvider) Cleanup(ctx context.Context, _ outbound.Sandbox) error {
	p.cleanupCalled = true
	p.cleanupContextErr = ctx.Err()

	return nil
}

type fakeModelClient struct{}

func (c fakeModelClient) Generate(context.Context, outbound.ModelRequest) (outbound.ModelResponse, error) {
	return outbound.ModelResponse{Content: "done"}, nil
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

type cancellingContextModelClient struct {
	cancel context.CancelFunc
}

func (c cancellingContextModelClient) Generate(context.Context, outbound.ModelRequest) (outbound.ModelResponse, error) {
	c.cancel()

	return outbound.ModelResponse{Content: "done"}, nil
}

type fakeGitProvider struct{}

func (p fakeGitProvider) Fetch(context.Context, outbound.GitFetchRequest) (outbound.GitCheckout, error) {
	return outbound.GitCheckout{Path: "/tmp/repo"}, nil
}

type fakePolicyDecision struct{}

func (p fakePolicyDecision) Decide(context.Context, outbound.PolicyDecisionRequest) (outbound.PolicyDecision, error) {
	return outbound.PolicyDecision{Allowed: true}, nil
}

type fakeSecretBroker struct{}

func (b fakeSecretBroker) Resolve(context.Context, outbound.SecretRequest) (outbound.SecretBundle, error) {
	return outbound.SecretBundle{Values: map[string]string{}}, nil
}

type recordingPublisher struct {
	events []outbound.Event
}

func (p *recordingPublisher) Publish(_ context.Context, event outbound.Event) error {
	p.events = append(p.events, event)

	return nil
}

var errPublishFailed = errors.New("publish failed")

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
