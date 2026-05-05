package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/po-sen/agentpool/internal/application/port/inbound"
)

const (
	maxJSONRequestBodyBytes      = 1 << 20
	maxMultipartRequestBodyBytes = 32 << 20

	headerContentType    = "Content-Type"
	messageRunIDRequired = "run id is required"
)

// Dependencies contains use cases required by the HTTP API.
type Dependencies struct {
	CreateRun        inbound.CreateRunUseCase
	GetRun           inbound.GetRunUseCase
	ListRuns         inbound.ListRunsUseCase
	CancelRun        inbound.CancelRunUseCase
	ListRunArtifacts inbound.ListRunArtifactsUseCase
	GetRunArtifact   inbound.GetRunArtifactUseCase
}

// NewRouter creates the AgentPool HTTP router.
func NewRouter(deps Dependencies) http.Handler {
	handler := &Handler{
		createRun:        deps.CreateRun,
		getRun:           deps.GetRun,
		listRuns:         deps.ListRuns,
		cancelRun:        deps.CancelRun,
		listRunArtifacts: deps.ListRunArtifacts,
		getRunArtifact:   deps.GetRunArtifact,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handler.health)
	mux.HandleFunc("POST /v1/runs", handler.create)
	mux.HandleFunc("GET /v1/runs", handler.list)
	mux.HandleFunc("GET /v1/runs/{id}", handler.get)
	mux.HandleFunc("GET /v1/runs/{id}/artifacts", handler.listArtifacts)
	mux.HandleFunc("GET /v1/runs/{id}/artifacts/{path...}", handler.getArtifact)
	mux.HandleFunc("POST /v1/runs/{id}/cancel", handler.cancel)

	return mux
}

// Handler owns HTTP request handling.
type Handler struct {
	createRun        inbound.CreateRunUseCase
	getRun           inbound.GetRunUseCase
	listRuns         inbound.ListRunsUseCase
	cancelRun        inbound.CancelRunUseCase
	listRunArtifacts inbound.ListRunArtifactsUseCase
	getRunArtifact   inbound.GetRunArtifactUseCase
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set(headerContentType, "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("ok\n")); err != nil {
		return
	}
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	command, err := decodeCreateRunCommand(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	created, err := h.createRun.CreateRun(r.Context(), command)
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
		writeError(w, http.StatusBadRequest, messageRunIDRequired)
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
		writeError(w, http.StatusBadRequest, messageRunIDRequired)
		return
	}

	cancelled, err := h.cancelRun.CancelRun(r.Context(), inbound.CancelRunCommand{RunID: id})
	if err != nil {
		writeApplicationError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toRunResponse(cancelled))
}

func (h *Handler) listArtifacts(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, messageRunIDRequired)
		return
	}
	if h.listRunArtifacts == nil {
		writeError(w, http.StatusInternalServerError, "artifact API is not configured")
		return
	}

	artifacts, err := h.listRunArtifacts.ListRunArtifacts(r.Context(), inbound.GetRunArtifactsQuery{RunID: id})
	if err != nil {
		writeApplicationError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toArtifactsResponse(artifacts))
}

func (h *Handler) getArtifact(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	artifactPath := strings.TrimPrefix(r.PathValue("path"), "/")
	if id == "" {
		writeError(w, http.StatusBadRequest, messageRunIDRequired)
		return
	}
	if artifactPath == "" {
		writeError(w, http.StatusBadRequest, "artifact path is required")
		return
	}
	if h.getRunArtifact == nil {
		writeError(w, http.StatusInternalServerError, "artifact API is not configured")
		return
	}

	artifact, err := h.getRunArtifact.GetRunArtifact(r.Context(), inbound.GetRunArtifactQuery{
		RunID: id,
		Path:  artifactPath,
	})
	if err != nil {
		writeApplicationError(w, err)
		return
	}

	contentType := artifact.MediaType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set(headerContentType, contentType)
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(artifact.Content); err != nil {
		return
	}
}

func writeApplicationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, inbound.ErrRunNotFound):
		writeError(w, http.StatusNotFound, "run not found")
	case errors.Is(err, inbound.ErrArtifactNotFound):
		writeError(w, http.StatusNotFound, "artifact not found")
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

func decodeCreateRunCommand(w http.ResponseWriter, r *http.Request) (inbound.CreateRunCommand, error) {
	contentType := r.Header.Get(headerContentType)
	if contentType == "" {
		return decodeJSONCreateRunCommand(w, r)
	}

	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return inbound.CreateRunCommand{}, errors.New("invalid Content-Type")
	}

	switch mediaType {
	case "application/json":
		return decodeJSONCreateRunCommand(w, r)
	case "multipart/form-data":
		return decodeMultipartCreateRunCommand(w, r, params["boundary"])
	default:
		return inbound.CreateRunCommand{}, errors.New("unsupported Content-Type")
	}
}

func decodeJSONCreateRunCommand(w http.ResponseWriter, r *http.Request) (inbound.CreateRunCommand, error) {
	var request createRunRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONRequestBodyBytes))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&request); err != nil {
		return inbound.CreateRunCommand{}, errors.New("invalid JSON request body")
	}

	var extra struct{}
	if err := decoder.Decode(&extra); err != io.EOF {
		return inbound.CreateRunCommand{}, errors.New("request body must contain a single JSON object")
	}

	return inbound.CreateRunCommand{
		ProjectID:     request.ProjectID,
		Prompt:        request.Prompt,
		RepositoryURL: request.RepositoryURL,
		Branch:        request.Branch,
	}, nil
}

func decodeMultipartCreateRunCommand(
	w http.ResponseWriter,
	r *http.Request,
	boundary string,
) (inbound.CreateRunCommand, error) {
	if boundary == "" {
		return inbound.CreateRunCommand{}, errors.New("multipart boundary is required")
	}

	reader := multipart.NewReader(http.MaxBytesReader(w, r.Body, maxMultipartRequestBodyBytes), boundary)
	var command inbound.CreateRunCommand
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return inbound.CreateRunCommand{}, errors.New("invalid multipart request body")
		}

		if err := applyCreateRunPart(&command, part); err != nil {
			_ = part.Close()

			return inbound.CreateRunCommand{}, err
		}
		_ = part.Close()
	}

	return command, nil
}

func applyCreateRunPart(command *inbound.CreateRunCommand, part *multipart.Part) error {
	name := part.FormName()
	switch name {
	case "project_id":
		value, err := readMultipartPart(part)
		if err != nil {
			return err
		}
		command.ProjectID = string(value)
	case "prompt":
		value, err := readMultipartPart(part)
		if err != nil {
			return err
		}
		command.Prompt = string(value)
	case "repository_url":
		value, err := readMultipartPart(part)
		if err != nil {
			return err
		}
		command.RepositoryURL = string(value)
	case "branch":
		value, err := readMultipartPart(part)
		if err != nil {
			return err
		}
		command.Branch = string(value)
	case "files":
		filename := uploadedFilename(part)
		if filename == "" {
			return errors.New("uploaded file filename is required")
		}
		content, err := readMultipartPart(part)
		if err != nil {
			return err
		}
		command.Attachments = append(command.Attachments, inbound.AttachmentInput{
			Filename:  filename,
			MediaType: part.Header.Get(headerContentType),
			Content:   content,
			SizeBytes: int64(len(content)),
		})
	default:
		return errors.New("unknown multipart field")
	}

	return nil
}

func readMultipartPart(part *multipart.Part) ([]byte, error) {
	content, err := io.ReadAll(part)
	if err != nil {
		return nil, errors.New("invalid multipart request body")
	}

	return content, nil
}

func uploadedFilename(part *multipart.Part) string {
	_, params, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
	if err == nil && params["filename"] != "" {
		return params["filename"]
	}

	return part.FileName()
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set(headerContentType, "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		return
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}
