package command

import (
	"context"
	"time"

	"github.com/po-sen/agentpool/internal/application/port/inbound"
	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

var _ inbound.CreateRunUseCase = (*CreateRunHandler)(nil)

// CreateRunHandler handles run creation commands.
type CreateRunHandler struct {
	repo   run.Repository
	queue  outbound.RunQueue
	events outbound.EventPublisher
	ids    outbound.IDGenerator
	clock  func() time.Time
}

// CreateRunOption configures a CreateRunHandler.
type CreateRunOption func(*CreateRunHandler)

// WithCreateRunClock injects a time source for tests.
func WithCreateRunClock(clock func() time.Time) CreateRunOption {
	return func(handler *CreateRunHandler) {
		handler.clock = clock
	}
}

// NewCreateRunHandler wires the create-run command handler.
func NewCreateRunHandler(
	repo run.Repository,
	queue outbound.RunQueue,
	events outbound.EventPublisher,
	ids outbound.IDGenerator,
	options ...CreateRunOption,
) *CreateRunHandler {
	handler := &CreateRunHandler{
		repo:   repo,
		queue:  queue,
		events: events,
		ids:    ids,
		clock: func() time.Time {
			return time.Now().UTC()
		},
	}

	for _, option := range options {
		option(handler)
	}

	return handler
}

// CreateRun creates, stores, queues, and publishes a new run.
func (h *CreateRunHandler) CreateRun(ctx context.Context, command inbound.CreateRunCommand) (inbound.RunView, error) {
	id, err := h.ids.NewRunID()
	if err != nil {
		return inbound.RunView{}, err
	}

	now := h.clock()
	attachments := make([]run.TaskAttachment, 0, len(command.Attachments))
	for _, attachment := range command.Attachments {
		sizeBytes := attachment.SizeBytes
		if sizeBytes == 0 {
			sizeBytes = int64(len(attachment.Content))
		}
		attachments = append(attachments, run.TaskAttachment{
			Filename:  attachment.Filename,
			MediaType: attachment.MediaType,
			Content:   append([]byte(nil), attachment.Content...),
			SizeBytes: sizeBytes,
		})
	}
	created, err := run.New(id, run.TaskSpec{
		ProjectID:     command.ProjectID,
		Prompt:        command.Prompt,
		RepositoryURL: command.RepositoryURL,
		Branch:        command.Branch,
		Attachments:   attachments,
	}, now)
	if err != nil {
		return inbound.RunView{}, inbound.NewInvalidInputError(err)
	}

	if err := h.repo.Save(ctx, created); err != nil {
		return inbound.RunView{}, err
	}
	if err := h.queue.Enqueue(ctx, created.ID); err != nil {
		return inbound.RunView{}, err
	}
	if err := h.events.Publish(ctx, outbound.Event{
		Type:       outbound.EventRunCreated,
		RunID:      created.ID,
		OccurredAt: now,
	}); err != nil {
		return inbound.RunView{}, err
	}

	return toRunView(created), nil
}
