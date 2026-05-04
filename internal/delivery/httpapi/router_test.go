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

type getRunStub struct{}

func (s *getRunStub) GetRun(context.Context, inbound.GetRunQuery) (inbound.RunView, error) {
	return inbound.RunView{}, nil
}

type cancelRunStub struct{}

func (s *cancelRunStub) CancelRun(context.Context, inbound.CancelRunCommand) (inbound.RunView, error) {
	return inbound.RunView{}, nil
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
