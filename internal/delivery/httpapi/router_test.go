package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/po-sen/agentpool/internal/application/port/inbound"
)

var errCreateRunInvalid = errors.New("invalid create run")

func TestCreateRunWithoutWorkspace(t *testing.T) {
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
	if create.command.Workspace.Type != "" {
		t.Fatalf("workspace type = %q, want empty", create.command.Workspace.Type)
	}
}

func TestCreateRunWithNoneWorkspace(t *testing.T) {
	create := &createRunStub{
		view: inbound.RunView{
			ID:     "run_test",
			Status: "queued",
			Task: inbound.TaskView{
				Prompt:    "do work",
				Workspace: inbound.WorkspaceSourceView{Type: "none"},
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

	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/runs",
		strings.NewReader(`{"prompt":"do work","workspace":{"type":"none"}}`),
	)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusCreated)
	}
	if create.command.Workspace.Type != "none" {
		t.Fatalf("workspace type = %q, want none", create.command.Workspace.Type)
	}
	if strings.Contains(response.Body.String(), `"workspace"`) {
		t.Fatalf("response contains none workspace: %s", response.Body.String())
	}
}

func TestCreateRunConfiguredWorkspaceReturnsBadRequest(t *testing.T) {
	create := &createRunStub{
		err: inbound.NewInvalidInputError(errCreateRunInvalid),
	}
	router := NewRouter(Dependencies{
		CreateRun: create,
		ListRuns:  &listRunsStub{},
		GetRun:    &getRunStub{},
		CancelRun: &cancelRunStub{},
	})

	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/runs",
		strings.NewReader(`{"prompt":"do work","workspace":{"type":"configured"}}`),
	)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestCreateRunWithSnapshotWorkspace(t *testing.T) {
	create := &createRunStub{
		view: inbound.RunView{
			ID:     "run_test",
			Status: "queued",
			Task: inbound.TaskView{
				Prompt: "do work",
				Workspace: inbound.WorkspaceSourceView{
					Type:       "snapshot",
					SnapshotID: "wsnap_test",
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

	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/runs",
		strings.NewReader(`{"prompt":"do work","workspace":{"type":"snapshot","snapshot_id":"wsnap_test"}}`),
	)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusCreated)
	}
	if create.command.Workspace.Type != "snapshot" {
		t.Fatalf("workspace type = %q, want snapshot", create.command.Workspace.Type)
	}
	if create.command.Workspace.SnapshotID != "wsnap_test" {
		t.Fatalf("snapshot id = %q, want wsnap_test", create.command.Workspace.SnapshotID)
	}
	if !strings.Contains(response.Body.String(), `"workspace":{"type":"snapshot","snapshot_id":"wsnap_test"}`) {
		t.Fatalf("response does not contain workspace: %s", response.Body.String())
	}
}

func TestCreateRunSnapshotWorkspaceWithoutSnapshotIDReturnsBadRequest(t *testing.T) {
	create := &createRunStub{
		err: inbound.NewInvalidInputError(errCreateRunInvalid),
	}
	router := NewRouter(Dependencies{
		CreateRun: create,
		ListRuns:  &listRunsStub{},
		GetRun:    &getRunStub{},
		CancelRun: &cancelRunStub{},
	})

	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/runs",
		strings.NewReader(`{"prompt":"do work","workspace":{"type":"snapshot"}}`),
	)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestCreateRunUnknownWorkspaceReturnsBadRequest(t *testing.T) {
	create := &createRunStub{
		err: inbound.NewInvalidInputError(errCreateRunInvalid),
	}
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
	called  bool
	command inbound.CreateRunCommand
	view    inbound.RunView
	err     error
}

func (s *createRunStub) CreateRun(_ context.Context, command inbound.CreateRunCommand) (inbound.RunView, error) {
	s.called = true
	s.command = command
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
