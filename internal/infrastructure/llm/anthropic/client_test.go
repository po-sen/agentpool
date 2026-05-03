package anthropic_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/infrastructure/llm/anthropic"
)

func TestClientGenerateSendsHeadersAndParsesText(t *testing.T) {
	var apiKey string
	var version string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("path = %s, want /v1/messages", r.URL.Path)
		}
		apiKey = r.Header.Get("x-api-key")
		version = r.Header.Get("anthropic-version")
		writeJSON(t, w, map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "done"},
			},
		})
	}))
	defer server.Close()

	client, err := anthropic.NewClient(anthropic.Config{
		BaseURL: server.URL,
		Model:   "claude-sonnet-4-5",
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
	if apiKey != "test-key" {
		t.Fatalf("x-api-key = %q, want test-key", apiKey)
	}
	if version != "2023-06-01" {
		t.Fatalf("anthropic-version = %q, want 2023-06-01", version)
	}
}

func TestNewClientRequiresAPIKey(t *testing.T) {
	_, err := anthropic.NewClient(anthropic.Config{
		BaseURL: "https://api.anthropic.com",
		Model:   "claude-sonnet-4-5",
	})
	if err == nil {
		t.Fatal("NewClient() error = nil, want missing API key error")
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

func TestClientGenerateHandlesNoText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]any{"content": []any{}})
	}))
	defer server.Close()

	client := newClient(t, server.URL)
	_, err := client.Generate(context.Background(), outbound.ModelRequest{})
	if err == nil {
		t.Fatal("Generate() error = nil, want error")
	}
}

func newClient(t *testing.T, baseURL string) *anthropic.Client {
	t.Helper()

	client, err := anthropic.NewClient(anthropic.Config{
		BaseURL: baseURL,
		Model:   "claude-sonnet-4-5",
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
