package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/po-sen/agentpool/internal/application/port/inbound"
)

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
	called bool
}

func (s *createRunStub) CreateRun(context.Context, inbound.CreateRunCommand) (inbound.RunView, error) {
	s.called = true

	return inbound.RunView{}, nil
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
