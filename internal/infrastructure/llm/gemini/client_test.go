package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
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

	client, err := NewClient(Config{
		BaseURL: server.URL,
		Model:   "gemini-2.5-flash",
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
	if response.RequestMessages[0].Role != "system_instruction" {
		t.Fatalf("RequestMessages[0].Role = %q, want system_instruction", response.RequestMessages[0].Role)
	}
	if !strings.Contains(response.RequestMessages[0].Content, "[REDACTED]") ||
		!strings.Contains(response.RequestMessages[0].Content, "Previous assistant attempt that failed validation:") ||
		!strings.Contains(response.RequestMessages[0].Content, "return JSON") {
		t.Fatalf("RequestMessages[0].Content = %q, want redacted system diagnostics", response.RequestMessages[0].Content)
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
	if len(requestBody.SystemInstruction.Parts) != 3 {
		t.Fatalf("len(system parts) = %d, want 3", len(requestBody.SystemInstruction.Parts))
	}
	if requestBody.SystemInstruction.Parts[0].Text != "follow protocol" {
		t.Fatalf("system text = %q, want follow protocol", requestBody.SystemInstruction.Parts[0].Text)
	}
	if requestBody.SystemInstruction.Parts[1].Text != "Previous assistant attempt that failed validation:\nplain text" {
		t.Fatalf("system attempt = %q, want assistant attempt", requestBody.SystemInstruction.Parts[1].Text)
	}
	if requestBody.SystemInstruction.Parts[2].Text != "return JSON" {
		t.Fatalf("system correction = %q, want return JSON", requestBody.SystemInstruction.Parts[2].Text)
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

func TestToGeminiContentsMapsFunctionCallAndResponse(t *testing.T) {
	contents := toGeminiContents([]outbound.ModelTurn{
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
	})

	if len(contents) != 2 {
		t.Fatalf("len(contents) = %d, want model functionCall and user functionResponse", len(contents))
	}
	if contents[0].Role != "model" || len(contents[0].Parts) != 1 || contents[0].Parts[0].FunctionCall == nil {
		t.Fatalf("contents[0] = %#v, want model functionCall", contents[0])
	}
	if contents[0].Parts[0].FunctionCall.ID != "call_1" ||
		contents[0].Parts[0].FunctionCall.Name != "sandbox_exec" ||
		contents[0].Parts[0].FunctionCall.Args["command"] != "echo hi" {
		t.Fatalf("functionCall = %#v, want sandbox_exec", contents[0].Parts[0].FunctionCall)
	}
	if contents[1].Role != "user" || len(contents[1].Parts) != 1 || contents[1].Parts[0].FunctionResponse == nil {
		t.Fatalf("contents[1] = %#v, want user functionResponse", contents[1])
	}
	if contents[1].Parts[0].FunctionResponse.ID != "call_1" ||
		contents[1].Parts[0].FunctionResponse.Name != "sandbox_exec" {
		t.Fatalf("functionResponse = %#v, want call_1 sandbox_exec", contents[1].Parts[0].FunctionResponse)
	}
}

func TestToGeminiToolsBuildsFunctionDeclarations(t *testing.T) {
	tools := toGeminiTools([]outbound.ToolDefinition{
		{
			Name:        "workspace",
			Description: "Lists workspace metadata.",
			Arguments: []outbound.ToolArgumentDefinition{
				{Name: "operation", Description: "Operation to run.", Required: true, Example: "list"},
			},
		},
	})

	if len(tools) != 1 || len(tools[0].FunctionDeclarations) != 1 {
		t.Fatalf("tools = %#v, want one function declaration", tools)
	}
	declaration := tools[0].FunctionDeclarations[0]
	if declaration.Name != "workspace" {
		t.Fatalf("declaration name = %q, want workspace", declaration.Name)
	}
	if declaration.Parameters.Properties["operation"].Type != "string" {
		t.Fatalf("operation parameter = %#v, want string", declaration.Parameters.Properties["operation"])
	}
}

func TestNewClientRequiresAPIKey(t *testing.T) {
	_, err := NewClient(Config{
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

func newClient(t *testing.T, baseURL string) *Client {
	t.Helper()

	client, err := NewClient(Config{
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
