package openai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/infrastructure/llm/openai"
)

func TestClientGenerateSendsAuthorizationAndParsesContent(t *testing.T) {
	var auth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s, want /v1/chat/completions", r.URL.Path)
		}
		auth = r.Header.Get("Authorization")
		writeJSON(t, w, map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "done"}},
			},
		})
	}))
	defer server.Close()

	client, err := openai.NewClient(openai.Config{
		BaseURL: server.URL + "/v1",
		Model:   "gpt-4.1-mini",
		APIKey:  "test-key",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	response, err := client.Generate(context.Background(), outbound.ModelRequest{
		Messages: []outbound.ModelMessage{{Role: "user", Content: "do work"}},
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
}

func TestNewClientRequiresAPIKey(t *testing.T) {
	_, err := openai.NewClient(openai.Config{
		BaseURL: "https://api.openai.com/v1",
		Model:   "gpt-4.1-mini",
	})
	if err == nil {
		t.Fatal("NewClient() error = nil, want missing API key error")
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

func newClient(t *testing.T, baseURL string) *openai.Client {
	t.Helper()

	client, err := openai.NewClient(openai.Config{
		BaseURL: baseURL,
		Model:   "gpt-4.1-mini",
		APIKey:  "test-key",
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
