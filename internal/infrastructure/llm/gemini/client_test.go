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
	var requestBody struct {
		SystemInstruction struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"system_instruction"`
		Contents []struct {
			Role  string `json:"role"`
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"contents"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key = r.Header.Get("x-goog-api-key")
		path = r.URL.Path
		rawQuery = r.URL.RawQuery
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
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
		Messages: []outbound.ModelMessage{
			{Role: "system", Content: "follow protocol"},
			{Role: "user", Content: "do work"},
			{Role: "assistant", Content: "thinking"},
		},
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
	if len(requestBody.SystemInstruction.Parts) != 1 {
		t.Fatalf("len(system parts) = %d, want 1", len(requestBody.SystemInstruction.Parts))
	}
	if requestBody.SystemInstruction.Parts[0].Text != "follow protocol" {
		t.Fatalf("system text = %q, want follow protocol", requestBody.SystemInstruction.Parts[0].Text)
	}
	if len(requestBody.Contents) != 2 {
		t.Fatalf("len(contents) = %d, want 2", len(requestBody.Contents))
	}
	if requestBody.Contents[0].Role != "user" {
		t.Fatalf("contents[0].Role = %q, want user", requestBody.Contents[0].Role)
	}
	if requestBody.Contents[1].Role != "model" {
		t.Fatalf("contents[1].Role = %q, want model", requestBody.Contents[1].Role)
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
