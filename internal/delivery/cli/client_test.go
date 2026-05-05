package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClientCreateRunJSONRequestBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != apiRunsPath {
			t.Fatalf("request = %s %s, want POST %s", request.Method, request.URL.Path, apiRunsPath)
		}
		if got := request.Header.Get(contentTypeHeader); got != jsonContentType {
			t.Fatalf("content type = %q, want %q", got, jsonContentType)
		}
		var body createRunJSONRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body.Prompt != "do work" {
			t.Fatalf("prompt = %q, want do work", body.Prompt)
		}
		writeRunTestResponse(writer, http.StatusCreated, RunResponse{ID: "run_json", Status: "queued"})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	response, err := client.CreateRunJSON(context.Background(), CreateRunRequest{Prompt: "do work"})
	if err != nil {
		t.Fatalf("create run json: %v", err)
	}
	if response.ID != "run_json" {
		t.Fatalf("run id = %q, want run_json", response.ID)
	}
}

func TestClientCreateRunMultipartRequest(t *testing.T) {
	filePath := writeTempFile(t, "README.md", "hello")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != apiRunsPath {
			t.Fatalf("request = %s %s, want POST %s", request.Method, request.URL.Path, apiRunsPath)
		}
		reader := newMultipartReader(t, request)
		assertMultipartPromptAndFile(t, reader)
		writeRunTestResponse(writer, http.StatusCreated, RunResponse{ID: "run_file", Status: "queued"})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	response, err := client.CreateRunMultipart(context.Background(), CreateRunRequest{
		Prompt: "inspect",
		Files:  []string{filePath},
	})
	if err != nil {
		t.Fatalf("create run multipart: %v", err)
	}
	if response.ID != "run_file" {
		t.Fatalf("run id = %q, want run_file", response.ID)
	}
}

func TestClientWaitRunPollsUntilCompleted(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests++
		status := "running"
		if requests == 2 {
			status = statusCompleted
		}
		writeRunTestResponse(writer, http.StatusOK, RunResponse{ID: "run_wait", Status: status})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	response, err := client.WaitRun(context.Background(), "run_wait", time.Millisecond)
	if err != nil {
		t.Fatalf("wait run: %v", err)
	}
	if response.Status != statusCompleted {
		t.Fatalf("status = %q, want %q", response.Status, statusCompleted)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
}

func TestClientWaitRunHonorsTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeRunTestResponse(writer, http.StatusOK, RunResponse{ID: "run_timeout", Status: "running"})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	_, err := client.WaitRun(ctx, "run_timeout", time.Hour)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait run error = %v, want deadline exceeded", err)
	}
}

func TestClientGetListCancelPaths(t *testing.T) {
	seen := make([]string, 0, 3)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seen = append(seen, request.Method+" "+request.URL.Path)
		switch request.URL.Path {
		case apiRunsPath:
			writeRunListTestResponse(writer, []RunResponse{{ID: "run_list", Status: "queued"}})
		case apiRunsPath + "/run_get":
			writeRunTestResponse(writer, http.StatusOK, RunResponse{ID: "run_get", Status: statusCompleted})
		case apiRunsPath + "/run_cancel/" + apiCancelPath:
			writeRunTestResponse(writer, http.StatusOK, RunResponse{ID: "run_cancel", Status: statusCancelled})
		default:
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	if _, err := client.GetRun(context.Background(), "run_get"); err != nil {
		t.Fatalf("get run: %v", err)
	}
	if _, err := client.ListRuns(context.Background()); err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if _, err := client.CancelRun(context.Background(), "run_cancel"); err != nil {
		t.Fatalf("cancel run: %v", err)
	}

	want := []string{"GET /v1/runs/run_get", "GET /v1/runs", "POST /v1/runs/run_cancel/cancel"}
	for index := range want {
		if seen[index] != want[index] {
			t.Fatalf("seen paths = %#v, want %#v", seen, want)
		}
	}
}

func TestUploadFilenamePreservesSafeRelativePath(t *testing.T) {
	got, err := uploadFilename("internal/application/workflow/worker.go")
	if err != nil {
		t.Fatalf("upload filename: %v", err)
	}
	if got != "internal/application/workflow/worker.go" {
		t.Fatalf("filename = %q, want relative path", got)
	}
}

func TestUploadFilenameUsesBaseForAbsolutePath(t *testing.T) {
	got, err := uploadFilename(filepath.Join(t.TempDir(), "README.md"))
	if err != nil {
		t.Fatalf("upload filename: %v", err)
	}
	if got != "README.md" {
		t.Fatalf("filename = %q, want base filename", got)
	}
}

func TestUploadFilenameRejectsUnsafeRelativePath(t *testing.T) {
	_, err := uploadFilename("../README.md")
	if err == nil {
		t.Fatal("upload filename error = nil, want error")
	}
}

func newTestClient(t *testing.T, addr string) *Client {
	t.Helper()

	client, err := NewClient(addr)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	return client
}

func writeTempFile(t *testing.T, name string, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	return path
}

func newMultipartReader(t *testing.T, request *http.Request) *multipart.Reader {
	t.Helper()

	mediaType, params, err := mime.ParseMediaType(request.Header.Get(contentTypeHeader))
	if err != nil {
		t.Fatalf("parse content type: %v", err)
	}
	if mediaType != "multipart/form-data" {
		t.Fatalf("media type = %q, want multipart/form-data", mediaType)
	}

	return multipart.NewReader(request.Body, params["boundary"])
}

func assertMultipartPromptAndFile(t *testing.T, reader *multipart.Reader) {
	t.Helper()

	part, err := reader.NextPart()
	if err != nil {
		t.Fatalf("read prompt part: %v", err)
	}
	prompt, err := io.ReadAll(part)
	if err != nil {
		t.Fatalf("read prompt: %v", err)
	}
	if part.FormName() != flagPrompt || string(prompt) != "inspect" {
		t.Fatalf("prompt part = %s %q, want prompt inspect", part.FormName(), string(prompt))
	}

	part, err = reader.NextPart()
	if err != nil {
		t.Fatalf("read file part: %v", err)
	}
	content, err := io.ReadAll(part)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if part.FormName() != multipartFilesName || !strings.HasSuffix(part.FileName(), "README.md") {
		t.Fatalf("file part = %s %s, want files README.md", part.FormName(), part.FileName())
	}
	if string(content) != "hello" {
		t.Fatalf("file content = %q, want hello", string(content))
	}
}

func writeRunTestResponse(writer http.ResponseWriter, status int, response RunResponse) {
	writer.Header().Set(contentTypeHeader, jsonContentType)
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(response); err != nil {
		panic(err)
	}
}

func writeRunListTestResponse(writer http.ResponseWriter, response []RunResponse) {
	writer.Header().Set(contentTypeHeader, jsonContentType)
	if err := json.NewEncoder(writer).Encode(response); err != nil {
		panic(err)
	}
}
