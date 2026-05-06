package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestClientGenerateSendsHeadersAndParsesText(t *testing.T) {
	var apiKey string
	var version string
	var requestBody struct {
		System   string `json:"system"`
		Messages []struct {
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("path = %s, want /v1/messages", r.URL.Path)
		}
		apiKey = r.Header.Get("x-api-key")
		version = r.Header.Get("anthropic-version")
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		writeJSON(t, w, map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "done"},
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL: server.URL,
		Model:   "claude-sonnet-4-5",
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
				},
			},
			{
				Role: outbound.ModelRoleAssistant,
				Parts: []outbound.ModelPart{
					{Kind: outbound.ModelPartKindAssistantResponse, Text: "thinking"},
				},
			},
			{
				Role: outbound.ModelRoleAssistant,
				Parts: []outbound.ModelPart{
					{Kind: outbound.ModelPartKindAssistantAttempt, Text: "plain text"},
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
	if response.RequestMessages[0].Role != "system" {
		t.Fatalf("RequestMessages[0].Role = %q, want system", response.RequestMessages[0].Role)
	}
	if !strings.Contains(response.RequestMessages[0].Content, "follow protocol") ||
		!strings.Contains(response.RequestMessages[0].Content, "Previous assistant attempt that failed validation:") ||
		!strings.Contains(response.RequestMessages[0].Content, "return JSON") {
		t.Fatalf("RequestMessages[0].Content = %q, want system diagnostics", response.RequestMessages[0].Content)
	}
	if apiKey != "test-key" {
		t.Fatalf("x-api-key = %q, want test-key", apiKey)
	}
	if version != "2023-06-01" {
		t.Fatalf("anthropic-version = %q, want 2023-06-01", version)
	}
	if requestBody.System != "follow protocol\n\nPrevious assistant attempt that failed validation:\nplain text\n\nreturn JSON" {
		t.Fatalf("system = %q, want instructions and correction", requestBody.System)
	}
	if len(requestBody.Messages) != 2 {
		t.Fatalf("len(messages) = %d, want 2", len(requestBody.Messages))
	}
	if requestBody.Messages[0].Role != "user" {
		t.Fatalf("messages[0].Role = %q, want user", requestBody.Messages[0].Role)
	}
	if requestBody.Messages[1].Role != "assistant" {
		t.Fatalf("messages[1].Role = %q, want assistant", requestBody.Messages[1].Role)
	}
}

func TestToAnthropicMessagesMapsToolUseAndResultBlocks(t *testing.T) {
	messages := toAnthropicMessages([]outbound.ModelTurn{
		{
			Role: outbound.ModelRoleAssistant,
			Parts: []outbound.ModelPart{
				{
					Kind:          outbound.ModelPartKindToolCall,
					ToolCallID:    "toolu_1",
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
					ToolCallID: "toolu_1",
					ToolName:   "sandbox_exec",
					Text:       "exit_code: 0\nstdout:\nhi",
				},
			},
		},
	})

	if len(messages) != 2 {
		t.Fatalf("len(messages) = %d, want assistant tool_use and user tool_result", len(messages))
	}
	if messages[0].Role != "assistant" || len(messages[0].Content) != 1 {
		t.Fatalf("messages[0] = %#v, want assistant content block", messages[0])
	}
	if messages[0].Content[0].Type != "tool_use" ||
		messages[0].Content[0].ID != "toolu_1" ||
		messages[0].Content[0].Name != "sandbox_exec" ||
		messages[0].Content[0].Input["command"] != "echo hi" {
		t.Fatalf("assistant block = %#v, want tool_use", messages[0].Content[0])
	}
	if messages[1].Role != "user" || len(messages[1].Content) != 1 {
		t.Fatalf("messages[1] = %#v, want user tool_result", messages[1])
	}
	if messages[1].Content[0].Type != "tool_result" || messages[1].Content[0].ToolUseID != "toolu_1" {
		t.Fatalf("tool result block = %#v, want tool_result", messages[1].Content[0])
	}
}

func TestToAnthropicToolsBuildsInputSchema(t *testing.T) {
	tools := toAnthropicTools([]outbound.ToolDefinition{
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
	if tools[0].Name != "workspace" {
		t.Fatalf("tool name = %q, want workspace", tools[0].Name)
	}
	if tools[0].InputSchema.Properties["operation"].Type != "string" {
		t.Fatalf("operation parameter = %#v, want string", tools[0].InputSchema.Properties["operation"])
	}
}

func TestNewClientRequiresAPIKey(t *testing.T) {
	_, err := NewClient(Config{
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

func newClient(t *testing.T, baseURL string) *Client {
	t.Helper()

	client, err := NewClient(Config{
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
