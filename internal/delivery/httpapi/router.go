package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/po-sen/agentpool/internal/application/port/inbound"
)

const maxRequestBodyBytes = 1 << 20

// Dependencies contains use cases required by the HTTP API.
type Dependencies struct {
	CreateRun inbound.CreateRunUseCase
	GetRun    inbound.GetRunUseCase
	ListRuns  inbound.ListRunsUseCase
	CancelRun inbound.CancelRunUseCase
}

// NewRouter creates the AgentPool HTTP router.
func NewRouter(deps Dependencies) http.Handler {
	handler := &Handler{
		createRun: deps.CreateRun,
		getRun:    deps.GetRun,
		listRuns:  deps.ListRuns,
		cancelRun: deps.CancelRun,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handler.health)
	mux.HandleFunc("POST /v1/runs", handler.create)
	mux.HandleFunc("GET /v1/runs", handler.list)
	mux.HandleFunc("GET /v1/runs/{id}", handler.get)
	mux.HandleFunc("POST /v1/runs/{id}/cancel", handler.cancel)

	return mux
}

// Handler owns HTTP request handling.
type Handler struct {
	createRun inbound.CreateRunUseCase
	getRun    inbound.GetRunUseCase
	listRuns  inbound.ListRunsUseCase
	cancelRun inbound.CancelRunUseCase
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("ok\n")); err != nil {
		return
	}
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var request createRunRequest
	if err := decodeCreateRunRequest(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON request body")
		return
	}

	created, err := h.createRun.CreateRun(r.Context(), inbound.CreateRunCommand{
		ProjectID:     request.ProjectID,
		Prompt:        request.Prompt,
		RepositoryURL: request.RepositoryURL,
		Branch:        request.Branch,
		Workspace: inbound.WorkspaceSourceInput{
			Type: request.Workspace.Type,
		},
	})
	if err != nil {
		writeApplicationError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toRunResponse(created))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	items, err := h.listRuns.ListRuns(r.Context())
	if err != nil {
		writeApplicationError(w, err)
		return
	}

	response := make([]runResponse, 0, len(items))
	for _, item := range items {
		response = append(response, toRunResponse(item))
	}

	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "run id is required")
		return
	}

	found, err := h.getRun.GetRun(r.Context(), inbound.GetRunQuery{RunID: id})
	if err != nil {
		writeApplicationError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toRunResponse(found))
}

func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "run id is required")
		return
	}

	cancelled, err := h.cancelRun.CancelRun(r.Context(), inbound.CancelRunCommand{RunID: id})
	if err != nil {
		writeApplicationError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toRunResponse(cancelled))
}

func writeApplicationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, inbound.ErrRunNotFound):
		writeError(w, http.StatusNotFound, "run not found")
	case errors.Is(err, inbound.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, inbound.ErrConflict):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, inbound.ErrApprovalNotImplemented):
		writeError(w, http.StatusNotImplemented, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func decodeCreateRunRequest(w http.ResponseWriter, r *http.Request, request *createRunRequest) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBodyBytes))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(request); err != nil {
		return err
	}

	var extra struct{}
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("request body must contain a single JSON object")
	}

	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		return
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}
