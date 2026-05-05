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
