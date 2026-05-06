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

func TestExecuteRunWaitsWhenTimeoutIsZero(t *testing.T) {
	getCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == apiRunsPath:
			writeRunTestResponse(writer, http.StatusCreated, RunResponse{ID: "run_no_timeout", Status: "queued"})
		case request.Method == http.MethodGet && request.URL.Path == apiRunsPath+"/run_no_timeout":
			getCount++
			response := RunResponse{ID: "run_no_timeout", Status: "running"}
			if getCount == 2 {
				response = RunResponse{
					ID:     "run_no_timeout",
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

func TestExecuteRunWatchPrintsTimeline(t *testing.T) {
	getCount := 0
	startedAt := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	endedAt := startedAt.Add(time.Second)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == apiRunsPath:
			writeRunTestResponse(writer, http.StatusCreated, RunResponse{
				ID:        "run_watch",
				Status:    "queued",
				CreatedAt: startedAt,
				UpdatedAt: startedAt,
			})
		case request.Method == http.MethodGet && request.URL.Path == apiRunsPath+"/run_watch":
			getCount++
			response := RunResponse{
				ID:        "run_watch",
				Status:    "running",
				UpdatedAt: startedAt.Add(500 * time.Millisecond),
				Steps: []StepResponse{
					{Name: "workspace", Status: "running", Message: "Preparing workspace", StartedAt: startedAt},
				},
			}
			if getCount == 2 {
				response = RunResponse{
					ID:        "run_watch",
					Status:    statusCompleted,
					Result:    &RunResultResponse{Summary: "done"},
					UpdatedAt: endedAt,
					AgentTurns: []AgentTurnResponse{
						{
							Index:       2,
							Status:      "tool_call",
							ActionType:  "tool_call",
							ToolName:    "sandbox_exec",
							Message:     "model requested tool call",
							RawResponse: `{"type":"tool_call","tool":"sandbox_exec","arguments":{"command":"wc -l /workspace/README.md"}}`,
							StartedAt:   startedAt,
							EndedAt:     endedAt,
						},
					},
					ToolCalls: []ToolCallResponse{
						{
							Name:      "sandbox_exec",
							Arguments: map[string]string{"command": "wc -l /workspace/README.md"},
							Result:    "12 /workspace/README.md",
							StartedAt: startedAt,
							EndedAt:   endedAt,
						},
					},
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
			Watch:        true,
			Timeout:      time.Second,
			PollInterval: time.Millisecond,
		},
	}, &output)
	if err != nil {
		t.Fatalf("execute run watch: %v", err)
	}
	for _, want := range []string{
		"agent [turn 2]",
		"sandbox_exec",
		"request:\n    command: wc -l /workspace/README.md",
		"response:\n    type: tool_call\n    tool: sandbox_exec",
		"message:\n    model requested tool call",
		"tool [tool 1]",
		"response:\n    12 /workspace/README.md",
		"done",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("watch output missing %q:\n%s", want, output.String())
		}
	}
	if strings.Contains(output.String(), "TIME                SOURCE") {
		t.Fatalf("watch output still uses table header:\n%s", output.String())
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

func TestExecuteWatchPollsExistingRun(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != apiRunsPath+"/run_existing" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		requests++
		status := "running"
		if requests == 2 {
			status = statusCompleted
		}
		writeRunTestResponse(writer, http.StatusOK, RunResponse{ID: "run_existing", Status: status})
	}))
	defer server.Close()

	var output bytes.Buffer
	err := Execute(context.Background(), Command{
		Kind:  CommandWatch,
		Addr:  server.URL,
		RunID: "run_existing",
		Run: RunOptions{
			Timeout:      time.Second,
			PollInterval: time.Millisecond,
		},
	}, &output)
	if err != nil {
		t.Fatalf("execute watch: %v", err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
	for _, want := range []string{"Run: run_existing", statusCompleted} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("watch output missing %q:\n%s", want, output.String())
		}
	}
	if strings.Contains(output.String(), "status=running") {
		t.Fatalf("watch output should hide non-debug running status:\n%s", output.String())
	}
	if strings.Contains(output.String(), " run run_existing ") {
		t.Fatalf("watch output should not print run rows with run IDs:\n%s", output.String())
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
