package gemini_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/infrastructure/llm/gemini"
)

func TestClientGenerateSendsAPIKeyAndParsesText(t *testing.T) {
	var key string
	var path string
	var rawQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key = r.Header.Get("x-goog-api-key")
		path = r.URL.Path
		rawQuery = r.URL.RawQuery
		writeJSON(t, w, map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"parts": []map[string]any{{"text": "done"}},
					},
				},
			},
		})
	}))
	defer server.Close()

	client, err := gemini.NewClient(gemini.Config{
		BaseURL: server.URL,
		Model:   "gemini-2.5-flash",
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
	if key != "test-key" {
		t.Fatalf("x-goog-api-key = %q, want test-key", key)
	}
	if path != "/models/gemini-2.5-flash:generateContent" {
		t.Fatalf("path = %s, want generateContent path", path)
	}
	if rawQuery != "" {
		t.Fatalf("raw query = %q, want empty query", rawQuery)
	}
}

func TestNewClientRequiresAPIKey(t *testing.T) {
	_, err := gemini.NewClient(gemini.Config{
		BaseURL: "https://generativelanguage.googleapis.com/v1beta",
		Model:   "gemini-2.5-flash",
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
		writeJSON(t, w, map[string]any{"candidates": []any{}})
	}))
	defer server.Close()

	client := newClient(t, server.URL)
	_, err := client.Generate(context.Background(), outbound.ModelRequest{})
	if err == nil {
		t.Fatal("Generate() error = nil, want error")
	}
}

func newClient(t *testing.T, baseURL string) *gemini.Client {
	t.Helper()

	client, err := gemini.NewClient(gemini.Config{
		BaseURL: baseURL,
		Model:   "gemini-2.5-flash",
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
