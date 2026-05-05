package query

import (
	"context"
	"errors"

	"github.com/po-sen/agentpool/internal/application/port/inbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

var _ inbound.ListRunArtifactsUseCase = (*RunArtifactsHandler)(nil)
var _ inbound.GetRunArtifactUseCase = (*RunArtifactsHandler)(nil)

// RunArtifactsHandler handles artifact lookup queries.
type RunArtifactsHandler struct {
	repo run.Repository
}

// NewRunArtifactsHandler wires the run artifact query handler.
func NewRunArtifactsHandler(repo run.Repository) *RunArtifactsHandler {
	return &RunArtifactsHandler{repo: repo}
}

// ListRunArtifacts returns artifact metadata for one run.
func (h *RunArtifactsHandler) ListRunArtifacts(
	ctx context.Context,
	query inbound.GetRunArtifactsQuery,
) ([]inbound.ArtifactView, error) {
	id := run.RunID(query.RunID)
	if id == "" {
		return nil, inbound.NewInvalidInputError(run.ErrEmptyRunID)
	}

	found, err := h.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, run.ErrRunNotFound) {
			return nil, inbound.ErrRunNotFound
		}

		return nil, err
	}

	return artifactViews(found.Artifacts), nil
}

// GetRunArtifact returns one artifact's metadata and content.
func (h *RunArtifactsHandler) GetRunArtifact(
	ctx context.Context,
	query inbound.GetRunArtifactQuery,
) (inbound.ArtifactContentView, error) {
	id := run.RunID(query.RunID)
	if id == "" {
		return inbound.ArtifactContentView{}, inbound.NewInvalidInputError(run.ErrEmptyRunID)
	}
	if err := run.ValidateArtifactPath(query.Path); err != nil {
		return inbound.ArtifactContentView{}, inbound.NewInvalidInputError(err)
	}

	found, err := h.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, run.ErrRunNotFound) {
			return inbound.ArtifactContentView{}, inbound.ErrRunNotFound
		}

		return inbound.ArtifactContentView{}, err
	}
	artifact, ok := found.ArtifactByPath(query.Path)
	if !ok {
		return inbound.ArtifactContentView{}, inbound.ErrArtifactNotFound
	}

	return inbound.ArtifactContentView{
		Path:      artifact.Path,
		MediaType: artifact.MediaType,
		Content:   append([]byte(nil), artifact.Content...),
		SizeBytes: artifact.SizeBytes,
	}, nil
}
