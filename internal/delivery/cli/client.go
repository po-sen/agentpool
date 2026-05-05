package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	apiRunsPath        = "/v1/runs"
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
	Prompt string
	Files  []string
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
	Index           int       `json:"index"`
	Status          string    `json:"status"`
	ActionType      string    `json:"action_type,omitempty"`
	ToolName        string    `json:"tool_name,omitempty"`
	Message         string    `json:"message,omitempty"`
	ResponsePreview string    `json:"response_preview,omitempty"`
	StartedAt       time.Time `json:"started_at"`
	EndedAt         time.Time `json:"ended_at"`
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
	if len(request.Files) > 0 {
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
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField(flagPrompt, request.Prompt); err != nil {
		return RunResponse{}, err
	}
	for _, filePath := range request.Files {
		if err := writeMultipartFile(writer, filePath); err != nil {
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

func writeMultipartFile(writer *multipart.Writer, filePath string) error {
	filename, err := uploadFilename(filePath)
	if err != nil {
		return err
	}
	source, err := os.Open(filePath) //nolint:gosec // CLI intentionally reads user-selected upload paths.
	if err != nil {
		return err
	}
	defer func() {
		_ = source.Close()
	}()

	part, err := writer.CreateFormFile(multipartFilesName, filename)
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, source); err != nil {
		return err
	}

	return nil
}

func uploadFilename(filePath string) (string, error) {
	if strings.TrimSpace(filePath) == "" {
		return "", errors.New("file path is required")
	}
	if filepath.IsAbs(filePath) {
		return safeBaseFilename(filePath)
	}

	name := filepath.ToSlash(filePath)
	if !safeRelativeFilename(name) {
		return "", fmt.Errorf("unsafe file path: %s", filePath)
	}

	return name, nil
}

func safeBaseFilename(filePath string) (string, error) {
	name := filepath.Base(filePath)
	if !safeRelativeFilename(name) {
		return "", fmt.Errorf("unsafe file path: %s", filePath)
	}

	return name, nil
}

func safeRelativeFilename(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") {
		return false
	}
	if path.Clean(name) != name {
		return false
	}
	for _, component := range strings.Split(name, "/") {
		if component == "" || component == "." || component == ".." {
			return false
		}
	}

	return true
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
