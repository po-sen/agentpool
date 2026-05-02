package query

import (
	"context"
	"errors"

	"github.com/po-sen/agentpool/internal/application/port/inbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

var _ inbound.GetRunUseCase = (*GetRunHandler)(nil)

// GetRunHandler handles run lookup queries.
type GetRunHandler struct {
	repo run.Repository
}

// NewGetRunHandler wires the get-run query handler.
func NewGetRunHandler(repo run.Repository) *GetRunHandler {
	return &GetRunHandler{
		repo: repo,
	}
}

// GetRun returns a run by ID.
func (h *GetRunHandler) GetRun(ctx context.Context, query inbound.GetRunQuery) (inbound.RunView, error) {
	id := run.RunID(query.RunID)
	if id == "" {
		return inbound.RunView{}, inbound.NewInvalidInputError(run.ErrEmptyRunID)
	}

	found, err := h.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, run.ErrRunNotFound) {
			return inbound.RunView{}, inbound.ErrRunNotFound
		}

		return inbound.RunView{}, err
	}

	return toRunView(found), nil
}
