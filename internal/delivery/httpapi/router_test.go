package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/po-sen/agentpool/internal/application/port/inbound"
)

func TestCreateRun(t *testing.T) {
	create := &createRunStub{
		view: inbound.RunView{
			ID:     "run_test",
			Status: "queued",
			Task:   inbound.TaskView{Prompt: "do work"},
			Steps:  []inbound.StepView{},
		},
	}
	router := NewRouter(Dependencies{
		CreateRun: create,
		ListRuns:  &listRunsStub{},
		GetRun:    &getRunStub{},
		CancelRun: &cancelRunStub{},
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/runs", strings.NewReader(`{"prompt":"do work"}`))
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusCreated)
	}
	if create.command.Prompt != "do work" {
		t.Fatalf("prompt = %q, want do work", create.command.Prompt)
	}
}

func TestCreateRunMultipartOneFile(t *testing.T) {
	create := &createRunStub{
		view: inbound.RunView{
			ID:     "run_test",
			Status: "queued",
			Task: inbound.TaskView{
				Prompt: "Inspect the uploaded file",
				Attachments: []inbound.AttachmentView{
					{Filename: "README.md", MediaType: "text/markdown", SizeBytes: 7},
				},
			},
			Steps: []inbound.StepView{},
		},
	}
	router := NewRouter(Dependencies{
		CreateRun: create,
		ListRuns:  &listRunsStub{},
		GetRun:    &getRunStub{},
		CancelRun: &cancelRunStub{},
	})
	request := newMultipartCreateRunRequest(t,
		map[string]string{"prompt": "Inspect the uploaded file"},
		[]uploadFile{{filename: "README.md", mediaType: "text/markdown", content: "# Demo\n"}},
	)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusCreated, response.Body.String())
	}
	if create.command.Prompt != "Inspect the uploaded file" {
		t.Fatalf("prompt = %q, want Inspect the uploaded file", create.command.Prompt)
	}
	if len(create.command.Attachments) != 1 {
		t.Fatalf("len(Attachments) = %d, want 1", len(create.command.Attachments))
	}
	if create.command.Attachments[0].Filename != "README.md" {
		t.Fatalf("filename = %q, want README.md", create.command.Attachments[0].Filename)
	}
	if string(create.command.Attachments[0].Content) != "# Demo\n" {
		t.Fatalf("content = %q, want # Demo", create.command.Attachments[0].Content)
	}
	if !strings.Contains(response.Body.String(), `"attachments":[{"filename":"README.md","media_type":"text/markdown","size_bytes":7}]`) {
		t.Fatalf("response missing attachment metadata: %s", response.Body.String())
	}
	if strings.Contains(response.Body.String(), "# Demo") {
		t.Fatalf("response leaked attachment content: %s", response.Body.String())
	}
}

func TestCreateRunMultipartMultipleFiles(t *testing.T) {
	create := &createRunStub{
		view: inbound.RunView{
			ID:     "run_test",
			Status: "queued",
			Task: inbound.TaskView{
				Prompt: "Inspect files",
				Attachments: []inbound.AttachmentView{
					{Filename: "README.md", MediaType: "text/markdown", SizeBytes: 7},
					{Filename: "notes.txt", MediaType: "text/plain", SizeBytes: 6},
				},
			},
			Steps: []inbound.StepView{},
		},
	}
	router := NewRouter(Dependencies{
		CreateRun: create,
		ListRuns:  &listRunsStub{},
		GetRun:    &getRunStub{},
		CancelRun: &cancelRunStub{},
	})
	request := newMultipartCreateRunRequest(t,
		map[string]string{"prompt": "Inspect files"},
		[]uploadFile{
			{filename: "README.md", mediaType: "text/markdown", content: "# Demo\n"},
			{filename: "notes.txt", mediaType: "text/plain", content: "notes\n"},
		},
	)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusCreated, response.Body.String())
	}
	if len(create.command.Attachments) != 2 {
		t.Fatalf("len(Attachments) = %d, want 2", len(create.command.Attachments))
	}
	if create.command.Attachments[1].Filename != "notes.txt" {
		t.Fatalf("second filename = %q, want notes.txt", create.command.Attachments[1].Filename)
	}
}

func TestCreateRunMultipartUnsafeFilenameRejected(t *testing.T) {
	create := &createRunStub{validate: rejectUnsafeAttachmentFilename}
	router := NewRouter(Dependencies{
		CreateRun: create,
		ListRuns:  &listRunsStub{},
		GetRun:    &getRunStub{},
		CancelRun: &cancelRunStub{},
	})
	request := newMultipartCreateRunRequest(t,
		map[string]string{"prompt": "Inspect files"},
		[]uploadFile{{filename: "../secret.txt", mediaType: "text/plain", content: "secret"}},
	)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestCreateRunMultipartUnsupportedFileRejected(t *testing.T) {
	create := &createRunStub{validate: rejectUnsupportedAttachment}
	router := NewRouter(Dependencies{
		CreateRun: create,
		ListRuns:  &listRunsStub{},
		GetRun:    &getRunStub{},
		CancelRun: &cancelRunStub{},
	})
	request := newMultipartCreateRunRequest(t,
		map[string]string{"prompt": "Inspect files"},
		[]uploadFile{{filename: "image.png", mediaType: "image/png", content: string([]byte{0xff, 0xfe})}},
	)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestCreateRunRejectsWorkspaceField(t *testing.T) {
	create := &createRunStub{}
	router := NewRouter(Dependencies{
		CreateRun: create,
		ListRuns:  &listRunsStub{},
		GetRun:    &getRunStub{},
		CancelRun: &cancelRunStub{},
	})

	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/runs",
		strings.NewReader(`{"prompt":"do work","workspace":{"type":"mounted"}}`),
	)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if create.called {
		t.Fatal("CreateRun was called for request with workspace field")
	}
}

func TestCreateRejectsTrailingJSON(t *testing.T) {
	create := &createRunStub{}
	router := NewRouter(Dependencies{
		CreateRun: create,
		ListRuns:  &listRunsStub{},
		GetRun:    &getRunStub{},
		CancelRun: &cancelRunStub{},
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/runs", strings.NewReader(`{"prompt":"do work"}{}`))
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if create.called {
		t.Fatal("CreateRun was called for malformed request body")
	}
}

func TestRunResponseOmitsZeroEndedAt(t *testing.T) {
	item := inbound.RunView{
		ID:        "run_test",
		Status:    "running",
		CreatedAt: time.Unix(100, 0).UTC(),
		UpdatedAt: time.Unix(101, 0).UTC(),
		Steps: []inbound.StepView{
			{
				Name:      "execute",
				Status:    "running",
				StartedAt: time.Unix(101, 0).UTC(),
			},
		},
	}

	payload, err := json.Marshal(toRunResponse(item))
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if strings.Contains(string(payload), "ended_at") {
		t.Fatalf("response contains unfinished step ended_at: %s", payload)
	}
}

func TestGetRunIncludesToolCalls(t *testing.T) {
	get := &getRunStub{
		view: inbound.RunView{
			ID:     "run_test",
			Status: "completed",
			ToolCalls: []inbound.ToolCallView{
				{
					Name:      "workspace",
					Arguments: map[string]string{"operation": "stat", "area": "input", "path": "README.md"},
					Result:    "virtual_path: /workspace/input/README.md\n",
					StartedAt: time.Unix(101, 0).UTC(),
					EndedAt:   time.Unix(102, 0).UTC(),
				},
			},
			Steps:     []inbound.StepView{},
			CreatedAt: time.Unix(100, 0).UTC(),
			UpdatedAt: time.Unix(103, 0).UTC(),
		},
	}
	router := NewRouter(Dependencies{
		CreateRun: &createRunStub{},
		ListRuns:  &listRunsStub{},
		GetRun:    get,
		CancelRun: &cancelRunStub{},
	})
	request := httptest.NewRequest(http.MethodGet, "/v1/runs/run_test", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"tool_calls":[{"name":"workspace"`) {
		t.Fatalf("response missing tool_calls: %s", response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"operation":"stat"`) {
		t.Fatalf("response missing tool arguments: %s", response.Body.String())
	}
}

func TestGetRunIncludesAgentTurns(t *testing.T) {
	get := &getRunStub{
		view: inbound.RunView{
			ID:     "run_test",
			Status: "completed",
			AgentTurns: []inbound.AgentTurnView{
				{
					Index:           1,
					Status:          "tool_call",
					ActionType:      "tool_call",
					ToolName:        "workspace",
					Message:         "model requested tool call",
					ResponsePreview: `{"type":"tool_call"}`,
					StartedAt:       time.Unix(101, 0).UTC(),
					EndedAt:         time.Unix(102, 0).UTC(),
				},
			},
			Steps:     []inbound.StepView{},
			CreatedAt: time.Unix(100, 0).UTC(),
			UpdatedAt: time.Unix(103, 0).UTC(),
		},
	}
	router := NewRouter(Dependencies{
		CreateRun: &createRunStub{},
		ListRuns:  &listRunsStub{},
		GetRun:    get,
		CancelRun: &cancelRunStub{},
	})
	request := httptest.NewRequest(http.MethodGet, "/v1/runs/run_test", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"agent_turns":[{"index":1,"status":"tool_call"`) {
		t.Fatalf("response missing agent_turns: %s", response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"tool_name":"workspace"`) {
		t.Fatalf("response missing turn tool name: %s", response.Body.String())
	}
}

func TestGetRunIncludesAgentSystemPrompt(t *testing.T) {
	get := &getRunStub{
		view: inbound.RunView{
			ID:                "run_test",
			Status:            "failed",
			AgentSystemPrompt: "AgentPool is running a task.\nAvailable tools:\n- none\n",
			Steps:             []inbound.StepView{},
			CreatedAt:         time.Unix(100, 0).UTC(),
			UpdatedAt:         time.Unix(101, 0).UTC(),
		},
	}
	router := NewRouter(Dependencies{
		CreateRun: &createRunStub{},
		ListRuns:  &listRunsStub{},
		GetRun:    get,
		CancelRun: &cancelRunStub{},
	})
	request := httptest.NewRequest(http.MethodGet, "/v1/runs/run_test", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"agent_system_prompt":"AgentPool is running a task.\nAvailable tools:\n- none\n"`) {
		t.Fatalf("response missing agent_system_prompt: %s", response.Body.String())
	}
}

func TestGetRunIncludesFailureDiagnosticsAndPartialToolCalls(t *testing.T) {
	get := &getRunStub{
		view: inbound.RunView{
			ID:             "run_test",
			Status:         "failed",
			FailureReason:  "run failed",
			FailureCode:    "tool_execution_failed",
			FailureMessage: "tool execution failed",
			AgentTurns: []inbound.AgentTurnView{
				{
					Index:     1,
					Status:    "tool_call",
					ToolName:  "sandbox_exec",
					Message:   "model requested tool call",
					StartedAt: time.Unix(100, 0).UTC(),
					EndedAt:   time.Unix(101, 0).UTC(),
				},
			},
			ToolCalls: []inbound.ToolCallView{
				{
					Name:      "sandbox_exec",
					Arguments: map[string]string{"command": "pwd"},
					Result:    "tool execution failed",
					IsError:   true,
					StartedAt: time.Unix(101, 0).UTC(),
					EndedAt:   time.Unix(102, 0).UTC(),
				},
			},
			Steps:     []inbound.StepView{},
			CreatedAt: time.Unix(100, 0).UTC(),
			UpdatedAt: time.Unix(103, 0).UTC(),
		},
	}
	router := NewRouter(Dependencies{
		CreateRun: &createRunStub{},
		ListRuns:  &listRunsStub{},
		GetRun:    get,
		CancelRun: &cancelRunStub{},
	})
	request := httptest.NewRequest(http.MethodGet, "/v1/runs/run_test", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, `"failure_code":"tool_execution_failed"`) {
		t.Fatalf("response missing failure_code: %s", body)
	}
	if !strings.Contains(body, `"failure_message":"tool execution failed"`) {
		t.Fatalf("response missing failure_message: %s", body)
	}
	if !strings.Contains(body, `"tool_calls":[{"name":"sandbox_exec"`) {
		t.Fatalf("response missing partial tool_calls: %s", body)
	}
	if !strings.Contains(body, `"agent_turns":[{"index":1,"status":"tool_call"`) {
		t.Fatalf("response missing partial agent_turns: %s", body)
	}
}

func TestGetRunIncludesArtifactMetadata(t *testing.T) {
	get := &getRunStub{
		view: inbound.RunView{
			ID:     "run_test",
			Status: "completed",
			Artifacts: []inbound.ArtifactView{
				{Path: "report.md", MediaType: "text/markdown", SizeBytes: 9},
			},
			Steps:     []inbound.StepView{},
			CreatedAt: time.Unix(100, 0).UTC(),
			UpdatedAt: time.Unix(101, 0).UTC(),
		},
	}
	router := NewRouter(Dependencies{
		CreateRun: &createRunStub{},
		ListRuns:  &listRunsStub{},
		GetRun:    get,
		CancelRun: &cancelRunStub{},
	})
	request := httptest.NewRequest(http.MethodGet, "/v1/runs/run_test", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"artifacts":[{"path":"report.md","media_type":"text/markdown","size_bytes":9}]`) {
		t.Fatalf("response missing artifact metadata: %s", response.Body.String())
	}
	if strings.Contains(response.Body.String(), "# Report") {
		t.Fatalf("response leaked artifact content: %s", response.Body.String())
	}
}

func TestListArtifacts(t *testing.T) {
	listArtifacts := &listArtifactsStub{
		artifacts: []inbound.ArtifactView{
			{Path: "report.md", MediaType: "text/markdown", SizeBytes: 9},
		},
	}
	router := NewRouter(Dependencies{
		CreateRun:        &createRunStub{},
		ListRuns:         &listRunsStub{},
		GetRun:           &getRunStub{},
		CancelRun:        &cancelRunStub{},
		ListRunArtifacts: listArtifacts,
	})
	request := httptest.NewRequest(http.MethodGet, "/v1/runs/run_test/artifacts", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	if listArtifacts.query.RunID != "run_test" {
		t.Fatalf("artifact list run id = %q, want run_test", listArtifacts.query.RunID)
	}
	if !strings.Contains(response.Body.String(), `"artifacts":[{"path":"report.md","media_type":"text/markdown","size_bytes":9}]`) {
		t.Fatalf("response missing artifact metadata: %s", response.Body.String())
	}
}

func TestGetArtifactContent(t *testing.T) {
	getArtifact := &getArtifactStub{
		artifact: inbound.ArtifactContentView{
			Path:      "reports/report.md",
			MediaType: "text/markdown",
			Content:   []byte("# Report\n"),
			SizeBytes: 9,
		},
	}
	router := NewRouter(Dependencies{
		CreateRun:      &createRunStub{},
		ListRuns:       &listRunsStub{},
		GetRun:         &getRunStub{},
		CancelRun:      &cancelRunStub{},
		GetRunArtifact: getArtifact,
	})
	request := httptest.NewRequest(http.MethodGet, "/v1/runs/run_test/artifacts/reports/report.md", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	if getArtifact.query.RunID != "run_test" || getArtifact.query.Path != "reports/report.md" {
		t.Fatalf("artifact query = %#v, want run_test reports/report.md", getArtifact.query)
	}
	if response.Header().Get(headerContentType) != "text/markdown" {
		t.Fatalf("content type = %q, want text/markdown", response.Header().Get(headerContentType))
	}
	if response.Body.String() != "# Report\n" {
		t.Fatalf("artifact body = %q, want report", response.Body.String())
	}
}

type createRunStub struct {
	called   bool
	command  inbound.CreateRunCommand
	view     inbound.RunView
	err      error
	validate func(inbound.CreateRunCommand) error
}

func (s *createRunStub) CreateRun(_ context.Context, command inbound.CreateRunCommand) (inbound.RunView, error) {
	s.called = true
	s.command = command
	if s.validate != nil {
		if err := s.validate(command); err != nil {
			return inbound.RunView{}, err
		}
	}
	if s.err != nil {
		return inbound.RunView{}, s.err
	}

	return s.view, nil
}

type listRunsStub struct{}

func (s *listRunsStub) ListRuns(context.Context) ([]inbound.RunView, error) {
	return nil, nil
}

type getRunStub struct {
	view inbound.RunView
	err  error
}

func (s *getRunStub) GetRun(context.Context, inbound.GetRunQuery) (inbound.RunView, error) {
	if s.err != nil {
		return inbound.RunView{}, s.err
	}

	return s.view, nil
}

type cancelRunStub struct{}

func (s *cancelRunStub) CancelRun(context.Context, inbound.CancelRunCommand) (inbound.RunView, error) {
	return inbound.RunView{}, nil
}

type listArtifactsStub struct {
	query     inbound.GetRunArtifactsQuery
	artifacts []inbound.ArtifactView
	err       error
}

func (s *listArtifactsStub) ListRunArtifacts(
	_ context.Context,
	query inbound.GetRunArtifactsQuery,
) ([]inbound.ArtifactView, error) {
	s.query = query
	if s.err != nil {
		return nil, s.err
	}

	return s.artifacts, nil
}

type getArtifactStub struct {
	query    inbound.GetRunArtifactQuery
	artifact inbound.ArtifactContentView
	err      error
}

func (s *getArtifactStub) GetRunArtifact(
	_ context.Context,
	query inbound.GetRunArtifactQuery,
) (inbound.ArtifactContentView, error) {
	s.query = query
	if s.err != nil {
		return inbound.ArtifactContentView{}, s.err
	}

	return s.artifact, nil
}

type uploadFile struct {
	filename  string
	mediaType string
	content   string
}

func newMultipartCreateRunRequest(
	t *testing.T,
	fields map[string]string,
	files []uploadFile,
) *http.Request {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatalf("write field: %v", err)
		}
	}
	for _, file := range files {
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="files"; filename="%s"`, file.filename))
		header.Set("Content-Type", file.mediaType)
		part, err := writer.CreatePart(header)
		if err != nil {
			t.Fatalf("create file part: %v", err)
		}
		if _, err := part.Write([]byte(file.content)); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/runs", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())

	return request
}

func rejectUnsafeAttachmentFilename(command inbound.CreateRunCommand) error {
	for _, attachment := range command.Attachments {
		if strings.Contains(attachment.Filename, "..") {
			return inbound.NewInvalidInputError(errors.New("unsafe filename"))
		}
	}

	return nil
}

func rejectUnsupportedAttachment(command inbound.CreateRunCommand) error {
	for _, attachment := range command.Attachments {
		if strings.HasSuffix(attachment.Filename, ".png") {
			return inbound.NewInvalidInputError(errors.New("unsupported attachment"))
		}
	}

	return nil
}
