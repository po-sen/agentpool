package command

import (
	"context"
	"errors"
	"time"

	"github.com/po-sen/agentpool/internal/application/port/inbound"
	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

var _ inbound.CancelRunUseCase = (*CancelRunHandler)(nil)

// CancelRunHandler handles run cancellation commands.
type CancelRunHandler struct {
	repo   outbound.RunRepository
	events outbound.EventPublisher
	clock  func() time.Time
}

// CancelRunOption configures a CancelRunHandler.
type CancelRunOption func(*CancelRunHandler)

// WithCancelRunClock injects a time source for tests.
func WithCancelRunClock(clock func() time.Time) CancelRunOption {
	return func(handler *CancelRunHandler) {
		handler.clock = clock
	}
}

// NewCancelRunHandler wires the cancel-run command handler.
func NewCancelRunHandler(
	repo outbound.RunRepository,
	events outbound.EventPublisher,
	options ...CancelRunOption,
) *CancelRunHandler {
	handler := &CancelRunHandler{
		repo:   repo,
		events: events,
		clock: func() time.Time {
			return time.Now().UTC()
		},
	}

	for _, option := range options {
		option(handler)
	}

	return handler
}

// CancelRun cancels a non-terminal run.
func (h *CancelRunHandler) CancelRun(ctx context.Context, command inbound.CancelRunCommand) (inbound.RunView, error) {
	id := run.RunID(command.RunID)
	if id == "" {
		return inbound.RunView{}, inbound.NewInvalidInputError(run.ErrEmptyRunID)
	}

	found, err := h.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, outbound.ErrRunNotFound) {
			return inbound.RunView{}, inbound.ErrRunNotFound
		}

		return inbound.RunView{}, err
	}

	now := h.clock()
	if err := found.Cancel(now); err != nil {
		return inbound.RunView{}, inbound.NewConflictError(err)
	}
	if err := h.repo.Save(ctx, found); err != nil {
		return inbound.RunView{}, err
	}
	if err := h.events.Publish(ctx, outbound.Event{
		Type:       outbound.EventRunCancelled,
		RunID:      found.ID,
		OccurredAt: now,
	}); err != nil {
		return inbound.RunView{}, err
	}

	return toRunView(found), nil
}
