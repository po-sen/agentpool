package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestClientGenerateSendsAuthorizationAndParsesContent(t *testing.T) {
	var auth string
	var requestBody struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s, want /v1/chat/completions", r.URL.Path)
		}
		auth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
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
		BaseURL: server.URL + "/v1",
		Model:   "gpt-4.1-mini",
		APIKey:  "test-key",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	response, err := client.Generate(context.Background(), outbound.ModelRequest{
		Instructions: "follow protocol",
		Turns: []outbound.ModelTurn{
			{
				Role: outbound.ModelRoleUser,
				Parts: []outbound.ModelPart{
					{Kind: outbound.ModelPartKindTaskPrompt, Text: "do work"},
					{Kind: outbound.ModelPartKindWorkspaceContext, Text: "files:\n- README.md"},
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
	if response.RequestMessages[0].Role != "developer" || response.RequestMessages[0].Content != "follow protocol" {
		t.Fatalf("RequestMessages[0] = %#v, want developer instructions", response.RequestMessages[0])
	}
	if response.RequestMessages[2].Role != "user" || response.RequestMessages[2].Content != "return JSON" {
		t.Fatalf("RequestMessages[2] = %#v, want user correction", response.RequestMessages[2])
	}
	if auth != "Bearer test-key" {
		t.Fatalf("authorization = %q, want bearer token", auth)
	}
	if len(requestBody.Messages) != 3 {
		t.Fatalf("len(messages) = %d, want developer, user, and user correction", len(requestBody.Messages))
	}
	if requestBody.Messages[0].Role != "developer" || requestBody.Messages[0].Content != "follow protocol" {
		t.Fatalf("messages[0] = %#v, want developer instructions", requestBody.Messages[0])
	}
	if requestBody.Messages[1].Role != "user" {
		t.Fatalf("messages[1].Role = %q, want user", requestBody.Messages[1].Role)
	}
	if !strings.Contains(requestBody.Messages[1].Content, "Workspace context:") {
		t.Fatalf("messages[1].Content = %q, want workspace context label", requestBody.Messages[1].Content)
	}
	if requestBody.Messages[2].Role != "user" || requestBody.Messages[2].Content != "return JSON" {
		t.Fatalf("messages[2] = %#v, want user correction", requestBody.Messages[2])
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

func TestToChatToolsBuildsFunctionSchemas(t *testing.T) {
	tools := toChatTools([]outbound.ToolDefinition{
		{
			Name:        "sandbox_exec",
			Description: "Runs a command.",
			Arguments: []outbound.ToolArgumentDefinition{
				{Name: "command", Description: "Command to run.", Required: true, Example: "pwd"},
			},
		},
	}, false)

	if len(tools) != 1 {
		t.Fatalf("len(tools) = %d, want 1", len(tools))
	}
	if tools[0].Type != "function" || tools[0].Function.Name != "sandbox_exec" {
		t.Fatalf("tool = %#v, want sandbox_exec function", tools[0])
	}
	if tools[0].Function.Parameters.Properties["command"].Type != "string" {
		t.Fatalf("command parameter = %#v, want string", tools[0].Function.Parameters.Properties["command"])
	}
	if tools[0].Function.Parameters.Properties["command"].Description != "Command to run." {
		t.Fatalf("command description = %q, want no example suffix", tools[0].Function.Parameters.Properties["command"].Description)
	}
	if len(tools[0].Function.Parameters.Required) != 1 || tools[0].Function.Parameters.Required[0] != "command" {
		t.Fatalf("required = %#v, want command", tools[0].Function.Parameters.Required)
	}
}

func TestToModelToolCallsParsesFunctionArguments(t *testing.T) {
	calls := toModelToolCalls([]chatToolCall{
		{
			ID:   "call_1",
			Type: "function",
			Function: chatToolCallFunction{
				Name:      "sandbox_exec",
				Arguments: `{"command":"echo hi","timeout_seconds":5,"dry_run":false}`,
			},
		},
	})

	if len(calls) != 1 {
		t.Fatalf("len(calls) = %d, want 1", len(calls))
	}
	if calls[0].ID != "call_1" || calls[0].Name != "sandbox_exec" {
		t.Fatalf("call = %#v, want sandbox_exec", calls[0])
	}
	if calls[0].Arguments["timeout_seconds"] != "5" || calls[0].Arguments["dry_run"] != "false" {
		t.Fatalf("arguments = %#v, want stringified scalars", calls[0].Arguments)
	}
}

func TestNewClientRequiresAPIKey(t *testing.T) {
	_, err := NewClient(Config{
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
	assertOpenAIRequestDiagnostics(t, response.RequestMessages)
}

func newClient(t *testing.T, baseURL string) *Client {
	t.Helper()

	client, err := NewClient(Config{
		BaseURL: baseURL,
		Model:   "gpt-4.1-mini",
		APIKey:  "test-key",
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

func assertOpenAIRequestDiagnostics(t *testing.T, messages []outbound.ModelRequestMessage) {
	t.Helper()

	if len(messages) != 3 {
		t.Fatalf("len(RequestMessages) = %d, want provider-facing diagnostics", len(messages))
	}
	if messages[0].Role != "developer" || messages[0].Content != "follow protocol" || messages[0].Kind != "" {
		t.Fatalf("RequestMessages[0] = %#v, want developer instructions without application kind", messages[0])
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
