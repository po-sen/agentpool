package openaicompatible

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

const defaultTimeout = 5 * time.Minute
const chatRoleSystem = "system"

// Config contains OpenAI-compatible model client configuration.
type Config struct {
	BaseURL string
	Model   string
	APIKey  string
	Timeout time.Duration
}

// Client calls an OpenAI-compatible chat completions API.
type Client struct {
	httpClient *http.Client
	baseURL    string
	model      string
	apiKey     string
}

var _ outbound.ModelClient = (*Client)(nil)

// NewClient creates an OpenAI-compatible model client.
func NewClient(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		return nil, errors.New("openai-compatible base URL is required")
	}
	if cfg.Model == "" {
		return nil, errors.New("openai-compatible model is required")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}

	return &Client{
		httpClient: &http.Client{Timeout: cfg.Timeout},
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		model:      cfg.Model,
		apiKey:     cfg.APIKey,
	}, nil
}

// Generate sends a chat completion request and returns generated content.
func (c *Client) Generate(ctx context.Context, req outbound.ModelRequest) (outbound.ModelResponse, error) {
	requestMessages := toModelRequestMessages(req)
	messages := toChatMessages(req)
	body, err := json.Marshal(chatCompletionRequest{
		Model:          c.model,
		Messages:       messages,
		Tools:          toChatTools(req.Tools),
		ResponseFormat: chatResponseFormat{Type: "json_object"},
		Temperature:    0,
		Stream:         false,
	})
	if err != nil {
		return outbound.ModelResponse{RequestMessages: requestMessages}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return outbound.ModelResponse{RequestMessages: requestMessages}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return outbound.ModelResponse{RequestMessages: requestMessages}, err
	}
	defer func() {
		_ = httpResp.Body.Close()
	}()

	if httpResp.StatusCode < http.StatusOK || httpResp.StatusCode >= http.StatusMultipleChoices {
		return outbound.ModelResponse{RequestMessages: requestMessages},
			fmt.Errorf("openai-compatible request failed: status %d: %s", httpResp.StatusCode, readBodySnippet(httpResp.Body))
	}

	var decoded chatCompletionResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&decoded); err != nil {
		return outbound.ModelResponse{RequestMessages: requestMessages}, fmt.Errorf("decode openai-compatible response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return outbound.ModelResponse{RequestMessages: requestMessages}, errors.New("openai-compatible response has no choices")
	}
	message := decoded.Choices[0].Message
	content := strings.TrimSpace(message.Content)
	toolCalls := toModelToolCalls(message.ToolCalls)
	if content == "" && len(toolCalls) == 0 {
		return outbound.ModelResponse{RequestMessages: requestMessages}, errors.New("openai-compatible response content is empty")
	}

	return outbound.ModelResponse{
		Content:         content,
		ToolCalls:       toolCalls,
		RequestMessages: requestMessages,
	}, nil
}

type chatCompletionRequest struct {
	Model          string             `json:"model"`
	Messages       []chatMessage      `json:"messages"`
	Tools          []chatTool         `json:"tools,omitempty"`
	ResponseFormat chatResponseFormat `json:"response_format"`
	Temperature    float64            `json:"temperature"`
	Stream         bool               `json:"stream"`
}

type chatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

type chatResponseFormat struct {
	Type string `json:"type"`
}

type chatTool struct {
	Type     string           `json:"type"`
	Function chatToolFunction `json:"function"`
}

type chatToolFunction struct {
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	Parameters  chatToolParameters `json:"parameters"`
}

type chatToolParameters struct {
	Type                 string                             `json:"type"`
	Properties           map[string]chatToolParameterSchema `json:"properties"`
	Required             []string                           `json:"required,omitempty"`
	AdditionalProperties bool                               `json:"additionalProperties"`
}

type chatToolParameterSchema struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

type chatToolCall struct {
	ID       string               `json:"id,omitempty"`
	Type     string               `json:"type,omitempty"`
	Function chatToolCallFunction `json:"function"`
}

type chatToolCallFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

func toChatMessages(request outbound.ModelRequest) []chatMessage {
	items := make([]chatMessage, 0, len(request.Turns)+1)
	if strings.TrimSpace(request.Instructions) != "" {
		items = append(items, chatMessage{
			Role:    chatRoleSystem,
			Content: request.Instructions,
		})
	}
	for _, turn := range request.Turns {
		items = appendChatMessages(items, turn)
	}

	return items
}

func toModelRequestMessages(request outbound.ModelRequest) []outbound.ModelRequestMessage {
	items := make([]outbound.ModelRequestMessage, 0, len(request.Turns)+1)
	if strings.TrimSpace(request.Instructions) != "" {
		items = append(items, outbound.ModelRequestMessage{
			Role:    chatRoleSystem,
			Content: request.Instructions,
		})
	}
	for _, message := range toChatMessages(outbound.ModelRequest{Turns: request.Turns}) {
		items = append(items, toModelRequestMessage(message))
	}

	return items
}

func appendChatMessages(items []chatMessage, turn outbound.ModelTurn) []chatMessage {
	if turn.Role == outbound.ModelRoleTool {
		for _, part := range turn.Parts {
			if part.Kind != outbound.ModelPartKindToolResult {
				continue
			}
			content := strings.TrimSpace(part.Text)
			if content == "" {
				continue
			}
			items = append(items, chatMessage{
				Role:       "tool",
				Content:    content,
				ToolCallID: strings.TrimSpace(part.ToolCallID),
			})
		}

		return items
	}

	content := joinModelTextParts(turn.Parts)
	toolCalls := toChatToolCallParts(turn.Parts)
	if content == "" && len(toolCalls) == 0 {
		return items
	}

	return append(items, chatMessage{
		Role:      toChatRole(turn.Role),
		Content:   content,
		ToolCalls: toolCalls,
	})
}

func toChatRole(role outbound.ModelRole) string {
	switch role {
	case outbound.ModelRoleAssistant:
		return "assistant"
	case outbound.ModelRoleRuntime:
		return "user"
	case outbound.ModelRoleTool:
		return "tool"
	default:
		return "user"
	}
}

func joinModelTextParts(parts []outbound.ModelPart) string {
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Kind == outbound.ModelPartKindToolCall || part.Kind == outbound.ModelPartKindToolResult {
			continue
		}
		text := strings.TrimSpace(part.Text)
		if text == "" {
			continue
		}
		if part.Kind == outbound.ModelPartKindWorkspaceContext {
			text = "Workspace context:\n" + text
		}
		items = append(items, text)
	}

	return strings.Join(items, "\n\n")
}

func toChatTools(tools []outbound.ToolDefinition) []chatTool {
	if len(tools) == 0 {
		return nil
	}

	items := make([]chatTool, 0, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		items = append(items, chatTool{
			Type: "function",
			Function: chatToolFunction{
				Name:        name,
				Description: strings.TrimSpace(tool.Description),
				Parameters:  toChatToolParameters(tool.Arguments),
			},
		})
	}

	return items
}

func toChatToolParameters(arguments []outbound.ToolArgumentDefinition) chatToolParameters {
	properties := map[string]chatToolParameterSchema{}
	required := []string{}
	for _, argument := range arguments {
		name := strings.TrimSpace(argument.Name)
		if name == "" {
			continue
		}
		description := strings.TrimSpace(argument.Description)
		if argument.Example != "" {
			description = strings.TrimSpace(description + " Example: " + argument.Example)
		}
		properties[name] = chatToolParameterSchema{
			Type:        "string",
			Description: description,
		}
		if argument.Required {
			required = append(required, name)
		}
	}

	return chatToolParameters{
		Type:                 "object",
		Properties:           properties,
		Required:             required,
		AdditionalProperties: false,
	}
}

func toChatToolCallParts(parts []outbound.ModelPart) []chatToolCall {
	items := make([]chatToolCall, 0)
	for _, part := range parts {
		if part.Kind != outbound.ModelPartKindToolCall {
			continue
		}
		name := strings.TrimSpace(part.ToolName)
		if name == "" {
			continue
		}
		items = append(items, chatToolCall{
			ID:   strings.TrimSpace(part.ToolCallID),
			Type: "function",
			Function: chatToolCallFunction{
				Name:      name,
				Arguments: encodeToolArguments(part.ToolArguments),
			},
		})
	}

	return items
}

func encodeToolArguments(arguments map[string]string) string {
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return "{}"
	}

	return string(encoded)
}

func toModelToolCalls(calls []chatToolCall) []outbound.ModelToolCall {
	if len(calls) == 0 {
		return nil
	}

	items := make([]outbound.ModelToolCall, 0, len(calls))
	for _, call := range calls {
		name := strings.TrimSpace(call.Function.Name)
		if name == "" {
			continue
		}
		items = append(items, outbound.ModelToolCall{
			ID:        strings.TrimSpace(call.ID),
			Name:      name,
			Arguments: decodeToolArguments(call.Function.Arguments),
		})
	}

	return items
}

func decodeToolArguments(value string) map[string]string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	decoder := json.NewDecoder(strings.NewReader(trimmed))
	decoder.UseNumber()
	var decoded map[string]interface{}
	if err := decoder.Decode(&decoded); err != nil {
		return nil
	}

	items := make(map[string]string, len(decoded))
	for key, value := range decoded {
		switch item := value.(type) {
		case string:
			items[key] = item
		case json.Number:
			items[key] = item.String()
		case bool:
			items[key] = fmt.Sprintf("%t", item)
		default:
			encoded, err := json.Marshal(item)
			if err == nil {
				items[key] = string(encoded)
			}
		}
	}

	return items
}

func toModelRequestMessage(message chatMessage) outbound.ModelRequestMessage {
	content := strings.TrimSpace(message.Content)
	if len(message.ToolCalls) > 0 {
		content = compactJSON(map[string]interface{}{
			"content":    content,
			"tool_calls": message.ToolCalls,
		})
	}

	return outbound.ModelRequestMessage{
		Role:       message.Role,
		Content:    content,
		ToolCallID: message.ToolCallID,
	}
}

func compactJSON(value interface{}) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}

	return string(encoded)
}

func readBodySnippet(reader io.Reader) string {
	body, err := io.ReadAll(io.LimitReader(reader, 1024))
	if err != nil {
		return "read response body failed"
	}

	return string(body)
}
