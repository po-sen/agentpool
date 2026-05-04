package openaicompatible

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestClientGenerateSendsChatCompletionRequest(t *testing.T) {
	var received map[string]any
	var auth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s, want /v1/chat/completions", r.URL.Path)
		}
		auth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		writeJSON(t, w, map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "done"}},
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL: server.URL + "/v1/",
		Model:   "local-model",
		APIKey:  "test-key",
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	response, err := client.Generate(context.Background(), outbound.ModelRequest{
		RunID: "run_test",
		Messages: []outbound.ModelMessage{
			{Role: "user", Content: "do work"},
		},
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if response.Content != "done" {
		t.Fatalf("content = %q, want done", response.Content)
	}
	if auth != "Bearer test-key" {
		t.Fatalf("authorization = %q, want bearer token", auth)
	}
	if received["model"] != "local-model" {
		t.Fatalf("model = %v, want local-model", received["model"])
	}
	if received["stream"] != false {
		t.Fatalf("stream = %v, want false", received["stream"])
	}
}

func TestClientGenerateOmitsAuthorizationWhenAPIKeyEmpty(t *testing.T) {
	var auth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		writeJSON(t, w, map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "done"}},
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL: server.URL,
		Model:   "local-model",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	if _, err := client.Generate(context.Background(), outbound.ModelRequest{}); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if auth != "" {
		t.Fatalf("authorization = %q, want empty", auth)
	}
}

func TestClientGenerateHandlesNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer server.Close()

	client := newClient(t, server.URL)
	_, err := client.Generate(context.Background(), outbound.ModelRequest{})
	if err == nil {
		t.Fatal("Generate() error = nil, want error")
	}
}

func TestClientGenerateHandlesNoChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]any{"choices": []any{}})
	}))
	defer server.Close()

	client := newClient(t, server.URL)
	_, err := client.Generate(context.Background(), outbound.ModelRequest{})
	if err == nil {
		t.Fatal("Generate() error = nil, want error")
	}
}

func TestNewClientRequiresBaseURLAndModel(t *testing.T) {
	if _, err := NewClient(Config{Model: "local-model"}); err == nil {
		t.Fatal("NewClient() missing base URL error = nil")
	}
	if _, err := NewClient(Config{BaseURL: "http://localhost"}); err == nil {
		t.Fatal("NewClient() missing model error = nil")
	}
}

func newClient(t *testing.T, baseURL string) *Client {
	t.Helper()

	client, err := NewClient(Config{
		BaseURL: baseURL,
		Model:   "local-model",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	return client
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
