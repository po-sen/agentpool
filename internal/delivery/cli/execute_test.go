package cli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestExecuteRunSubmitsPollsAndPrintsResult(t *testing.T) {
	getCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == apiRunsPath:
			writeRunTestResponse(writer, http.StatusCreated, RunResponse{ID: "run_exec", Status: "queued"})
		case request.Method == http.MethodGet && request.URL.Path == apiRunsPath+"/run_exec":
			getCount++
			response := RunResponse{ID: "run_exec", Status: "running"}
			if getCount == 2 {
				response = RunResponse{
					ID:     "run_exec",
					Status: statusCompleted,
					Result: &RunResultResponse{Summary: "done"},
				}
			}
			writeRunTestResponse(writer, http.StatusOK, response)
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	var output bytes.Buffer
	err := Execute(context.Background(), Command{
		Kind: CommandRun,
		Addr: server.URL,
		Run: RunOptions{
			Prompt:       "do work",
			Wait:         true,
			Timeout:      time.Second,
			PollInterval: time.Millisecond,
		},
	}, &output)
	if err != nil {
		t.Fatalf("execute run: %v", err)
	}
	if !strings.Contains(output.String(), "Result:\ndone") {
		t.Fatalf("output missing completed result: %s", output.String())
	}
}

func TestExecuteRunNoWaitPrintsCreatedRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != apiRunsPath {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		writeRunTestResponse(writer, http.StatusCreated, RunResponse{ID: "run_nowait", Status: "queued"})
	}))
	defer server.Close()

	var output bytes.Buffer
	err := Execute(context.Background(), Command{
		Kind: CommandRun,
		Addr: server.URL,
		Run: RunOptions{
			Prompt: "queue",
			Wait:   false,
		},
	}, &output)
	if err != nil {
		t.Fatalf("execute run: %v", err)
	}
	if !strings.Contains(output.String(), "Run: run_nowait") {
		t.Fatalf("output missing created run: %s", output.String())
	}
}

func TestExecuteGetWritesJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != apiRunsPath+"/run_get" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		writeRunTestResponse(writer, http.StatusOK, RunResponse{ID: "run_get", Status: statusCompleted})
	}))
	defer server.Close()

	var output bytes.Buffer
	err := Execute(context.Background(), Command{
		Kind:   CommandGet,
		Addr:   server.URL,
		RunID:  "run_get",
		Output: OutputOptions{JSON: true},
	}, &output)
	if err != nil {
		t.Fatalf("execute get: %v", err)
	}
	if !strings.Contains(output.String(), `"id": "run_get"`) {
		t.Fatalf("json output missing run id: %s", output.String())
	}
}

func TestExecuteArtifactsPrintsList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != apiRunsPath+"/run_artifacts/"+apiArtifactsPath {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		writer.Header().Set(contentTypeHeader, jsonContentType)
		if _, err := writer.Write([]byte(`{"artifacts":[{"path":"report.md","media_type":"text/markdown","size_bytes":9}]}`)); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer server.Close()

	var output bytes.Buffer
	err := Execute(context.Background(), Command{
		Kind:  CommandArtifacts,
		Addr:  server.URL,
		RunID: "run_artifacts",
	}, &output)
	if err != nil {
		t.Fatalf("execute artifacts: %v", err)
	}
	if !strings.Contains(output.String(), "- report.md (9 bytes, text/markdown)") {
		t.Fatalf("output missing artifact list: %s", output.String())
	}
}

func TestExecuteArtifactWritesContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != apiRunsPath+"/run_artifacts/"+apiArtifactsPath+"/report.md" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		writer.Header().Set(contentTypeHeader, "text/markdown")
		if _, err := writer.Write([]byte("# Report\n")); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer server.Close()

	var output bytes.Buffer
	err := Execute(context.Background(), Command{
		Kind:  CommandArtifact,
		Addr:  server.URL,
		RunID: "run_artifacts",
		Path:  "report.md",
	}, &output)
	if err != nil {
		t.Fatalf("execute artifact: %v", err)
	}
	if output.String() != "# Report\n" {
		t.Fatalf("artifact output = %q, want report", output.String())
	}
}
