package query

import (
	"context"

	"github.com/po-sen/agentpool/internal/application/port/inbound"
	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

var _ inbound.ListRunsUseCase = (*ListRunsHandler)(nil)

// ListRunsHandler handles run list queries.
type ListRunsHandler struct {
	reader outbound.RunReader
}

// NewListRunsHandler wires the list-runs query handler.
func NewListRunsHandler(reader outbound.RunReader) *ListRunsHandler {
	return &ListRunsHandler{
		reader: reader,
	}
}

// ListRuns returns all known runs.
func (h *ListRunsHandler) ListRuns(ctx context.Context) ([]inbound.RunView, error) {
	items, err := h.reader.List(ctx)
	if err != nil {
		return nil, err
	}

	views := make([]inbound.RunView, 0, len(items))
	for _, item := range items {
		views = append(views, toRunView(item))
	}

	return views, nil
}
