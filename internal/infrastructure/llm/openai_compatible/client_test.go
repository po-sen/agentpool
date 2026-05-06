package openaicompatible

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
		RunID:        "run_test",
		Instructions: "follow protocol",
		Turns: []outbound.ModelTurn{
			{
				Role: outbound.ModelRoleUser,
				Parts: []outbound.ModelPart{
					{Kind: outbound.ModelPartKindTaskPrompt, Text: "do work"},
				},
			},
			{
				Role: outbound.ModelRoleRuntime,
				Parts: []outbound.ModelPart{
					{Kind: outbound.ModelPartKindProtocolCorrection, Text: "return JSON"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if response.Content != "done" {
		t.Fatalf("content = %q, want done", response.Content)
	}
	if len(response.RequestMessages) != 3 {
		t.Fatalf("len(RequestMessages) = %d, want 3", len(response.RequestMessages))
	}
	if response.RequestMessages[0].Role != "system" || response.RequestMessages[0].Content != "follow protocol" {
		t.Fatalf("RequestMessages[0] = %#v, want system instructions", response.RequestMessages[0])
	}
	if response.RequestMessages[2].Role != "user" || response.RequestMessages[2].Content != "return JSON" {
		t.Fatalf("RequestMessages[2] = %#v, want user correction", response.RequestMessages[2])
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
	if received["temperature"] != float64(0) {
		t.Fatalf("temperature = %v, want 0", received["temperature"])
	}
	responseFormat, ok := received["response_format"].(map[string]any)
	if !ok || responseFormat["type"] != "json_object" {
		t.Fatalf("response_format = %#v, want json_object", received["response_format"])
	}
	messages, ok := received["messages"].([]any)
	if !ok || len(messages) != 3 {
		t.Fatalf("messages = %#v, want system, user, and user correction messages", received["messages"])
	}
	system, ok := messages[0].(map[string]any)
	if !ok || system["role"] != "system" || system["content"] != "follow protocol" {
		t.Fatalf("messages[0] = %#v, want system instructions", messages[0])
	}
	correction, ok := messages[2].(map[string]any)
	if !ok || correction["role"] != "user" || correction["content"] != "return JSON" {
		t.Fatalf("messages[2] = %#v, want user correction", messages[2])
	}
}

func TestToChatMessagesMapsNativeToolCallsAndResults(t *testing.T) {
	messages := toChatMessages(outbound.ModelRequest{
		Turns: []outbound.ModelTurn{
			{
				Role: outbound.ModelRoleAssistant,
				Parts: []outbound.ModelPart{
					{
						Kind:          outbound.ModelPartKindToolCall,
						ToolCallID:    "call_1",
						ToolName:      "sandbox_exec",
						ToolArguments: map[string]string{"command": "echo hi"},
					},
				},
			},
			{
				Role: outbound.ModelRoleTool,
				Parts: []outbound.ModelPart{
					{
						Kind:       outbound.ModelPartKindToolResult,
						ToolCallID: "call_1",
						ToolName:   "sandbox_exec",
						Text:       "exit_code: 0\nstdout:\nhi",
					},
				},
			},
		},
	})

	if len(messages) != 2 {
		t.Fatalf("len(messages) = %d, want assistant tool call and tool result", len(messages))
	}
	if messages[0].Role != "assistant" || len(messages[0].ToolCalls) != 1 {
		t.Fatalf("messages[0] = %#v, want assistant tool_calls", messages[0])
	}
	if messages[0].ToolCalls[0].ID != "call_1" ||
		messages[0].ToolCalls[0].Function.Name != "sandbox_exec" ||
		messages[0].ToolCalls[0].Function.Arguments != `{"command":"echo hi"}` {
		t.Fatalf("tool_calls[0] = %#v, want function arguments", messages[0].ToolCalls[0])
	}
	if messages[1].Role != "tool" || messages[1].ToolCallID != "call_1" {
		t.Fatalf("messages[1] = %#v, want tool role with tool_call_id", messages[1])
	}
}

func TestToChatMessagesMapsLegacyToolObservationAsUserText(t *testing.T) {
	messages := toChatMessages(outbound.ModelRequest{
		Turns: []outbound.ModelTurn{
			{
				Role: outbound.ModelRoleAssistant,
				Parts: []outbound.ModelPart{
					{Kind: outbound.ModelPartKindAssistantResponse, Text: `{"type":"tool_call","tool":"workspace"}`},
				},
			},
			{
				Role: outbound.ModelRoleRuntime,
				Parts: []outbound.ModelPart{
					{Kind: outbound.ModelPartKindToolObservation, Text: "Tool result for workspace:\nstaged"},
				},
			},
		},
	})

	if len(messages) != 2 {
		t.Fatalf("len(messages) = %d, want assistant text and user observation", len(messages))
	}
	if messages[0].Role != "assistant" || len(messages[0].ToolCalls) != 0 {
		t.Fatalf("messages[0] = %#v, want assistant text without native tool_calls", messages[0])
	}
	if messages[1].Role != "user" || !strings.Contains(messages[1].Content, "Tool result for workspace") {
		t.Fatalf("messages[1] = %#v, want user observation text", messages[1])
	}
}

func TestToChatToolsBuildsCompatibleFunctionSchemas(t *testing.T) {
	tools := toChatTools([]outbound.ToolDefinition{
		{
			Name:        "workspace",
			Description: "Lists workspace metadata.",
			Arguments: []outbound.ToolArgumentDefinition{
				{Name: "operation", Description: "Operation to run.", Required: true, Example: "list"},
			},
		},
	})

	if len(tools) != 1 {
		t.Fatalf("len(tools) = %d, want 1", len(tools))
	}
	if tools[0].Type != "function" || tools[0].Function.Name != "workspace" {
		t.Fatalf("tool = %#v, want workspace function", tools[0])
	}
	if tools[0].Function.Parameters.Properties["operation"].Type != "string" {
		t.Fatalf("operation parameter = %#v, want string", tools[0].Function.Parameters.Properties["operation"])
	}
	if tools[0].Function.Parameters.Properties["operation"].Description != "Operation to run." {
		t.Fatalf("operation description = %q, want no example suffix", tools[0].Function.Parameters.Properties["operation"].Description)
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
	response, err := client.Generate(context.Background(), diagnosticModelRequest())
	if err == nil {
		t.Fatal("Generate() error = nil, want error")
	}
	assertCompatibleRequestDiagnostics(t, response.RequestMessages)
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

func diagnosticModelRequest() outbound.ModelRequest {
	return outbound.ModelRequest{
		Instructions: "follow protocol",
		Turns: []outbound.ModelTurn{
			{
				Role: outbound.ModelRoleUser,
				Parts: []outbound.ModelPart{
					{Kind: outbound.ModelPartKindTaskPrompt, Text: "do work"},
				},
			},
			{
				Role: outbound.ModelRoleRuntime,
				Parts: []outbound.ModelPart{
					{Kind: outbound.ModelPartKindProtocolCorrection, Text: "return JSON"},
				},
			},
		},
	}
}

func assertCompatibleRequestDiagnostics(t *testing.T, messages []outbound.ModelRequestMessage) {
	t.Helper()

	if len(messages) != 3 {
		t.Fatalf("len(RequestMessages) = %d, want provider-facing diagnostics", len(messages))
	}
	if messages[0].Role != "system" || messages[0].Content != "follow protocol" || messages[0].Kind != "" {
		t.Fatalf("RequestMessages[0] = %#v, want system instructions without application kind", messages[0])
	}
	if messages[1].Role != "user" || messages[1].Content != "do work" || messages[1].Kind != "" {
		t.Fatalf("RequestMessages[1] = %#v, want user task without application kind", messages[1])
	}
	if messages[2].Role != "user" || messages[2].Content != "return JSON" || messages[2].Kind != "" {
		t.Fatalf("RequestMessages[2] = %#v, want user correction without application kind", messages[2])
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
