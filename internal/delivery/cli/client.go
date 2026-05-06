package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	apiRunsPath        = "/v1/runs"
	apiArtifactsPath   = "artifacts"
	apiCancelPath      = "cancel"
	contentTypeHeader  = "Content-Type"
	jsonContentType    = "application/json"
	multipartFilesName = "files"

	statusCompleted = "completed"
	statusFailed    = "failed"
	statusCancelled = "cancelled"
)

// Client is a small HTTP client for the public AgentPool API.
type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
}

// CreateRunRequest contains CLI run submission input.
type CreateRunRequest struct {
	Prompt   string
	Files    []string
	Dirs     []string
	Archives []string
}

// RunResponse mirrors the public run JSON response.
type RunResponse struct {
	ID                string              `json:"id"`
	Status            string              `json:"status"`
	Task              TaskResponse        `json:"task"`
	Result            *RunResultResponse  `json:"result,omitempty"`
	FailureReason     string              `json:"failure_reason,omitempty"`
	FailureCode       string              `json:"failure_code,omitempty"`
	FailureMessage    string              `json:"failure_message,omitempty"`
	Steps             []StepResponse      `json:"steps"`
	ToolCalls         []ToolCallResponse  `json:"tool_calls,omitempty"`
	AgentTurns        []AgentTurnResponse `json:"agent_turns,omitempty"`
	Artifacts         []ArtifactResponse  `json:"artifacts,omitempty"`
	AgentSystemPrompt string              `json:"agent_system_prompt,omitempty"`
	CreatedAt         time.Time           `json:"created_at"`
	UpdatedAt         time.Time           `json:"updated_at"`
}

// TaskResponse mirrors task metadata in the public run JSON response.
type TaskResponse struct {
	ProjectID     string               `json:"project_id,omitempty"`
	Prompt        string               `json:"prompt"`
	RepositoryURL string               `json:"repository_url,omitempty"`
	Branch        string               `json:"branch,omitempty"`
	Attachments   []AttachmentResponse `json:"attachments,omitempty"`
}

// AttachmentResponse mirrors uploaded file metadata in the public run JSON response.
type AttachmentResponse struct {
	Filename  string `json:"filename"`
	MediaType string `json:"media_type,omitempty"`
	SizeBytes int64  `json:"size_bytes"`
}

// ArtifactsResponse mirrors the public artifact list JSON response.
type ArtifactsResponse struct {
	Artifacts []ArtifactResponse `json:"artifacts"`
}

// ArtifactResponse mirrors artifact metadata in the public run JSON response.
type ArtifactResponse struct {
	Path      string `json:"path"`
	MediaType string `json:"media_type,omitempty"`
	SizeBytes int64  `json:"size_bytes"`
}

// ArtifactContent contains one fetched artifact body and response metadata.
type ArtifactContent struct {
	Path      string
	MediaType string
	Content   []byte
}

// RunResultResponse mirrors successful run output in the public run JSON response.
type RunResultResponse struct {
	Summary string `json:"summary,omitempty"`
}

// StepResponse mirrors one run lifecycle step in the public run JSON response.
type StepResponse struct {
	Name      string     `json:"name"`
	Status    string     `json:"status"`
	Message   string     `json:"message,omitempty"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}

// ToolCallResponse mirrors one tool call in the public run JSON response.
type ToolCallResponse struct {
	Name      string            `json:"name"`
	Arguments map[string]string `json:"arguments"`
	Result    string            `json:"result"`
	IsError   bool              `json:"is_error"`
	StartedAt time.Time         `json:"started_at"`
	EndedAt   time.Time         `json:"ended_at"`
}

// AgentTurnResponse mirrors one agent turn diagnostic in the public run JSON response.
type AgentTurnResponse struct {
	Index             int               `json:"index"`
	Status            string            `json:"status"`
	ActionType        string            `json:"action_type,omitempty"`
	ToolName          string            `json:"tool_name,omitempty"`
	Message           string            `json:"message,omitempty"`
	RequestMessages   []MessageResponse `json:"request_messages,omitempty"`
	RawResponse       string            `json:"raw_response,omitempty"`
	ResponseFormat    string            `json:"response_format,omitempty"`
	ProtocolErrorCode string            `json:"protocol_error_code,omitempty"`
	CorrectionMessage string            `json:"correction_message,omitempty"`
	ResponsePreview   string            `json:"response_preview,omitempty"`
	StartedAt         time.Time         `json:"started_at"`
	EndedAt           time.Time         `json:"ended_at"`
}

// MessageResponse mirrors one provider-neutral model request message in run diagnostics.
type MessageResponse struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type createRunJSONRequest struct {
	Prompt string `json:"prompt"`
}

type apiErrorResponse struct {
	Error string `json:"error"`
}

// NewClient creates an HTTP API client.
func NewClient(addr string) (*Client, error) {
	if strings.TrimSpace(addr) == "" {
		addr = defaultCLIAddr
	}
	parsed, err := url.Parse(strings.TrimRight(addr, "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid API address: %s", addr)
	}

	return &Client{
		baseURL:    parsed,
		httpClient: http.DefaultClient,
	}, nil
}

// CreateRun submits a prompt-only JSON run or multipart file run.
func (c *Client) CreateRun(ctx context.Context, request CreateRunRequest) (RunResponse, error) {
	if len(request.Files) > 0 || len(request.Dirs) > 0 || len(request.Archives) > 0 {
		return c.CreateRunMultipart(ctx, request)
	}

	return c.CreateRunJSON(ctx, request)
}

// CreateRunJSON submits a prompt-only run through POST /v1/runs.
func (c *Client) CreateRunJSON(ctx context.Context, request CreateRunRequest) (RunResponse, error) {
	body, err := json.Marshal(createRunJSONRequest{Prompt: request.Prompt})
	if err != nil {
		return RunResponse{}, err
	}
	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.endpoint(apiRunsPath),
		bytes.NewReader(body),
	)
	if err != nil {
		return RunResponse{}, err
	}
	httpRequest.Header.Set(contentTypeHeader, jsonContentType)

	var response RunResponse
	if err := c.doJSON(httpRequest, http.StatusCreated, &response); err != nil {
		return RunResponse{}, err
	}

	return response, nil
}

// CreateRunMultipart submits a run with uploaded files through POST /v1/runs.
func (c *Client) CreateRunMultipart(ctx context.Context, request CreateRunRequest) (RunResponse, error) {
	uploads, err := collectUploadInputs(request)
	if err != nil {
		return RunResponse{}, err
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField(flagPrompt, request.Prompt); err != nil {
		return RunResponse{}, err
	}
	for _, upload := range uploads {
		if err := writeMultipartUpload(writer, upload); err != nil {
			_ = writer.Close()

			return RunResponse{}, err
		}
	}
	if err := writer.Close(); err != nil {
		return RunResponse{}, err
	}

	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.endpoint(apiRunsPath),
		&body,
	)
	if err != nil {
		return RunResponse{}, err
	}
	httpRequest.Header.Set(contentTypeHeader, writer.FormDataContentType())

	var response RunResponse
	if err := c.doJSON(httpRequest, http.StatusCreated, &response); err != nil {
		return RunResponse{}, err
	}

	return response, nil
}

// GetRun fetches one run by ID.
func (c *Client) GetRun(ctx context.Context, id string) (RunResponse, error) {
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, c.runEndpoint(id), nil)
	if err != nil {
		return RunResponse{}, err
	}

	var response RunResponse
	if err := c.doJSON(httpRequest, http.StatusOK, &response); err != nil {
		return RunResponse{}, err
	}

	return response, nil
}

// ListRuns fetches all known runs.
func (c *Client) ListRuns(ctx context.Context) ([]RunResponse, error) {
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint(apiRunsPath), nil)
	if err != nil {
		return nil, err
	}

	var response []RunResponse
	if err := c.doJSON(httpRequest, http.StatusOK, &response); err != nil {
		return nil, err
	}

	return response, nil
}

// CancelRun cancels one run by ID.
func (c *Client) CancelRun(ctx context.Context, id string) (RunResponse, error) {
	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.runEndpoint(id)+"/"+apiCancelPath,
		nil,
	)
	if err != nil {
		return RunResponse{}, err
	}

	var response RunResponse
	if err := c.doJSON(httpRequest, http.StatusOK, &response); err != nil {
		return RunResponse{}, err
	}

	return response, nil
}

// ListArtifacts fetches artifact metadata for one run.
func (c *Client) ListArtifacts(ctx context.Context, id string) (ArtifactsResponse, error) {
	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		c.runEndpoint(id)+"/"+apiArtifactsPath,
		nil,
	)
	if err != nil {
		return ArtifactsResponse{}, err
	}

	var response ArtifactsResponse
	if err := c.doJSON(httpRequest, http.StatusOK, &response); err != nil {
		return ArtifactsResponse{}, err
	}

	return response, nil
}

// GetArtifact fetches one run artifact body.
func (c *Client) GetArtifact(ctx context.Context, id string, artifactPath string) (ArtifactContent, error) {
	if !safeRelativeFilename(artifactPath) {
		return ArtifactContent{}, fmt.Errorf("unsafe artifact path: %s", artifactPath)
	}
	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		c.runEndpoint(id)+"/"+apiArtifactsPath+"/"+artifactPathEndpoint(artifactPath),
		nil,
	)
	if err != nil {
		return ArtifactContent{}, err
	}

	response, err := c.httpClient.Do(httpRequest) //nolint:gosec // CLI intentionally connects to user-configured AgentPool API addresses.
	if err != nil {
		return ArtifactContent{}, err
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode != http.StatusOK {
		return ArtifactContent{}, decodeAPIError(response)
	}
	content, err := io.ReadAll(response.Body)
	if err != nil {
		return ArtifactContent{}, err
	}

	return ArtifactContent{
		Path:      artifactPath,
		MediaType: response.Header.Get(contentTypeHeader),
		Content:   content,
	}, nil
}

// WaitRun polls one run until it reaches a terminal state.
func (c *Client) WaitRun(ctx context.Context, id string, pollInterval time.Duration) (RunResponse, error) {
	if pollInterval <= 0 {
		pollInterval = defaultPollInterval
	}

	for {
		response, err := c.GetRun(ctx, id)
		if err != nil {
			return RunResponse{}, err
		}
		if response.Terminal() {
			return response, nil
		}
		if err := waitForPoll(ctx, pollInterval); err != nil {
			return RunResponse{}, err
		}
	}
}

// Terminal reports whether the run status is terminal.
func (r RunResponse) Terminal() bool {
	return r.Status == statusCompleted || r.Status == statusFailed || r.Status == statusCancelled
}

func (c *Client) endpoint(pathValue string) string {
	clone := *c.baseURL
	clone.Path = strings.TrimRight(c.baseURL.Path, "/") + pathValue

	return clone.String()
}

func (c *Client) runEndpoint(id string) string {
	return c.endpoint(apiRunsPath + "/" + url.PathEscape(id))
}

func artifactPathEndpoint(artifactPath string) string {
	components := strings.Split(artifactPath, "/")
	for index, component := range components {
		components[index] = url.PathEscape(component)
	}

	return strings.Join(components, "/")
}

func (c *Client) doJSON(request *http.Request, expectedStatus int, target any) error {
	response, err := c.httpClient.Do(request) //nolint:gosec // CLI intentionally connects to user-configured AgentPool API addresses.
	if err != nil {
		return err
	}
	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode != expectedStatus {
		return decodeAPIError(response)
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		return err
	}

	return nil
}

func decodeAPIError(response *http.Response) error {
	var payload apiErrorResponse
	body := io.LimitReader(response.Body, 4<<10)
	if err := json.NewDecoder(body).Decode(&payload); err == nil && payload.Error != "" {
		return fmt.Errorf("AgentPool API error (%d): %s", response.StatusCode, payload.Error)
	}

	return fmt.Errorf("AgentPool API error (%d)", response.StatusCode)
}

func writeMultipartUpload(writer *multipart.Writer, upload uploadFile) error {
	part, err := writer.CreateFormFile(multipartFilesName, upload.filename)
	if err != nil {
		return err
	}
	if _, err := part.Write(upload.content); err != nil {
		return err
	}

	return nil
}

func waitForPoll(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
