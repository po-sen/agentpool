package anthropic

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

const defaultTimeout = 30 * time.Second
const anthropicVersion = "2023-06-01"

// Config contains Anthropic model client configuration.
type Config struct {
	BaseURL string
	Model   string
	APIKey  string
	Timeout time.Duration
}

// Client calls the Anthropic Messages API.
type Client struct {
	httpClient *http.Client
	baseURL    string
	model      string
	apiKey     string
}

var _ outbound.ModelClient = (*Client)(nil)

// NewClient creates an Anthropic model client.
func NewClient(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		return nil, errors.New("anthropic base URL is required")
	}
	if cfg.Model == "" {
		return nil, errors.New("anthropic model is required")
	}
	if cfg.APIKey == "" {
		return nil, errors.New("anthropic API key is required")
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

// Generate sends a Messages API request and returns generated content.
func (c *Client) Generate(ctx context.Context, req outbound.ModelRequest) (outbound.ModelResponse, error) {
	system := toAnthropicSystem(req)
	messages := toAnthropicMessages(req.Turns)
	body, err := json.Marshal(messagesRequest{
		Model:     c.model,
		MaxTokens: 1024,
		System:    system,
		Messages:  messages,
		Tools:     toAnthropicTools(req.Tools),
	})
	if err != nil {
		return outbound.ModelResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return outbound.ModelResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return outbound.ModelResponse{}, err
	}
	defer func() {
		_ = httpResp.Body.Close()
	}()

	if httpResp.StatusCode < http.StatusOK || httpResp.StatusCode >= http.StatusMultipleChoices {
		return outbound.ModelResponse{}, fmt.Errorf("anthropic request failed: status %d: %s", httpResp.StatusCode, readBodySnippet(httpResp.Body))
	}

	var decoded messagesResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&decoded); err != nil {
		return outbound.ModelResponse{}, fmt.Errorf("decode anthropic response: %w", err)
	}
	texts := []string{}
	toolCalls := []outbound.ModelToolCall{}
	for _, block := range decoded.Content {
		text := strings.TrimSpace(block.Text)
		if block.Type == "text" && text != "" {
			texts = append(texts, text)
			continue
		}
		if block.Type == "tool_use" {
			name := strings.TrimSpace(block.Name)
			if name == "" {
				continue
			}
			toolCalls = append(toolCalls, outbound.ModelToolCall{
				ID:        strings.TrimSpace(block.ID),
				Name:      name,
				Arguments: stringifyToolInput(block.Input),
			})
		}
	}
	if len(texts) > 0 || len(toolCalls) > 0 {
		return outbound.ModelResponse{
			Content:         strings.Join(texts, "\n\n"),
			ToolCalls:       toolCalls,
			RequestMessages: toModelRequestMessages(req),
		}, nil
	}

	return outbound.ModelResponse{}, errors.New("anthropic response has no text content")
}

type messagesRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
}

type anthropicMessage struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

type messagesResponse struct {
	Content []struct {
		Type  string                 `json:"type"`
		Text  string                 `json:"text"`
		ID    string                 `json:"id"`
		Name  string                 `json:"name"`
		Input map[string]interface{} `json:"input"`
	} `json:"content"`
}

type anthropicContentBlock struct {
	Type      string            `json:"type"`
	Text      string            `json:"text,omitempty"`
	ID        string            `json:"id,omitempty"`
	Name      string            `json:"name,omitempty"`
	Input     map[string]string `json:"input,omitempty"`
	ToolUseID string            `json:"tool_use_id,omitempty"`
	Content   string            `json:"content,omitempty"`
	IsError   bool              `json:"is_error,omitempty"`
}

type anthropicTool struct {
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	InputSchema anthropicInputSchema `json:"input_schema"`
}

type anthropicInputSchema struct {
	Type                 string                              `json:"type"`
	Properties           map[string]anthropicParameterSchema `json:"properties"`
	Required             []string                            `json:"required,omitempty"`
	AdditionalProperties bool                                `json:"additionalProperties"`
}

type anthropicParameterSchema struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

func toAnthropicMessages(turns []outbound.ModelTurn) []anthropicMessage {
	items := make([]anthropicMessage, 0, len(turns))
	for _, turn := range turns {
		if turn.Role == outbound.ModelRoleRuntime || isAssistantAttemptTurn(turn) {
			continue
		}
		content := toAnthropicContentBlocks(turn)
		if len(content) == 0 {
			continue
		}

		items = appendOrMergeAnthropicMessage(items, anthropicMessage{
			Role:    toAnthropicRole(turn.Role),
			Content: content,
		})
	}

	return items
}

func toModelRequestMessages(request outbound.ModelRequest) []outbound.ModelRequestMessage {
	items := []outbound.ModelRequestMessage{}
	if system := toAnthropicSystemDiagnostics(request); system != "" {
		items = append(items, outbound.ModelRequestMessage{
			Role:    "system",
			Content: system,
		})
	}
	for _, message := range toAnthropicMessages(request.Turns) {
		items = append(items, outbound.ModelRequestMessage{
			Role:    message.Role,
			Content: anthropicBlocksDiagnostic(message.Content),
		})
	}

	return items
}

func toAnthropicSystemDiagnostics(request outbound.ModelRequest) string {
	parts := []string{}
	if strings.TrimSpace(request.Instructions) != "" {
		parts = append(parts, "[REDACTED]")
	}
	for _, turn := range request.Turns {
		switch {
		case turn.Role == outbound.ModelRoleRuntime:
			if content := joinModelParts(turn.Parts); content != "" {
				parts = append(parts, content)
			}
		case isAssistantAttemptTurn(turn):
			if content := joinModelParts(turn.Parts); content != "" {
				parts = append(parts, "Previous assistant attempt that failed validation:\n"+content)
			}
		}
	}

	return strings.Join(parts, "\n\n")
}

func toAnthropicSystem(request outbound.ModelRequest) string {
	parts := []string{}
	if instructions := strings.TrimSpace(request.Instructions); instructions != "" {
		parts = append(parts, instructions)
	}
	for _, turn := range request.Turns {
		switch {
		case turn.Role == outbound.ModelRoleRuntime:
			if content := joinModelParts(turn.Parts); content != "" {
				parts = append(parts, content)
			}
		case isAssistantAttemptTurn(turn):
			if content := joinModelParts(turn.Parts); content != "" {
				parts = append(parts, "Previous assistant attempt that failed validation:\n"+content)
			}
		}
	}

	return strings.Join(parts, "\n\n")
}

func isAssistantAttemptTurn(turn outbound.ModelTurn) bool {
	if turn.Role != outbound.ModelRoleAssistant || len(turn.Parts) == 0 {
		return false
	}
	for _, part := range turn.Parts {
		if part.Kind != outbound.ModelPartKindAssistantAttempt {
			return false
		}
	}

	return true
}

func toAnthropicRole(role outbound.ModelRole) string {
	switch role {
	case outbound.ModelRoleAssistant:
		return "assistant"
	case outbound.ModelRoleTool:
		return "user"
	default:
		return "user"
	}
}

func appendOrMergeAnthropicMessage(items []anthropicMessage, item anthropicMessage) []anthropicMessage {
	if len(items) == 0 || items[len(items)-1].Role != item.Role {
		return append(items, item)
	}

	items[len(items)-1].Content = append(items[len(items)-1].Content, item.Content...)

	return items
}

func toAnthropicContentBlocks(turn outbound.ModelTurn) []anthropicContentBlock {
	blocks := []anthropicContentBlock{}
	for _, part := range turn.Parts {
		switch part.Kind {
		case outbound.ModelPartKindToolCall:
			name := strings.TrimSpace(part.ToolName)
			if name == "" {
				continue
			}
			blocks = append(blocks, anthropicContentBlock{
				Type:  "tool_use",
				ID:    strings.TrimSpace(part.ToolCallID),
				Name:  name,
				Input: copyArguments(part.ToolArguments),
			})
		case outbound.ModelPartKindToolResult:
			content := strings.TrimSpace(part.Text)
			if content == "" {
				continue
			}
			blocks = append(blocks, anthropicContentBlock{
				Type:      "tool_result",
				ToolUseID: strings.TrimSpace(part.ToolCallID),
				Content:   content,
				IsError:   part.IsError,
			})
		default:
			text := modelPartText(part)
			if text == "" {
				continue
			}
			blocks = append(blocks, anthropicContentBlock{
				Type: "text",
				Text: text,
			})
		}
	}

	return blocks
}

func joinModelParts(parts []outbound.ModelPart) string {
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Kind == outbound.ModelPartKindToolCall || part.Kind == outbound.ModelPartKindToolResult {
			continue
		}
		text := modelPartText(part)
		if text == "" {
			continue
		}
		items = append(items, text)
	}

	return strings.Join(items, "\n\n")
}

func modelPartText(part outbound.ModelPart) string {
	text := strings.TrimSpace(part.Text)
	if text == "" {
		return ""
	}
	if part.Kind == outbound.ModelPartKindWorkspaceContext {
		text = "Workspace context:\n" + text
	}

	return text
}

func toAnthropicTools(tools []outbound.ToolDefinition) []anthropicTool {
	if len(tools) == 0 {
		return nil
	}

	items := make([]anthropicTool, 0, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		items = append(items, anthropicTool{
			Name:        name,
			Description: strings.TrimSpace(tool.Description),
			InputSchema: toAnthropicInputSchema(tool.Arguments),
		})
	}

	return items
}

func toAnthropicInputSchema(arguments []outbound.ToolArgumentDefinition) anthropicInputSchema {
	properties := map[string]anthropicParameterSchema{}
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
		properties[name] = anthropicParameterSchema{
			Type:        "string",
			Description: description,
		}
		if argument.Required {
			required = append(required, name)
		}
	}

	return anthropicInputSchema{
		Type:                 "object",
		Properties:           properties,
		Required:             required,
		AdditionalProperties: false,
	}
}

func stringifyToolInput(input map[string]interface{}) map[string]string {
	if len(input) == 0 {
		return nil
	}

	items := make(map[string]string, len(input))
	for key, value := range input {
		switch item := value.(type) {
		case string:
			items[key] = item
		case float64:
			items[key] = fmt.Sprintf("%v", item)
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

func copyArguments(arguments map[string]string) map[string]string {
	if len(arguments) == 0 {
		return nil
	}

	copied := make(map[string]string, len(arguments))
	for key, value := range arguments {
		copied[key] = value
	}

	return copied
}

func anthropicBlocksDiagnostic(blocks []anthropicContentBlock) string {
	if len(blocks) == 0 {
		return ""
	}
	texts := []string{}
	for _, block := range blocks {
		switch block.Type {
		case "text":
			if text := strings.TrimSpace(block.Text); text != "" {
				texts = append(texts, text)
			}
		default:
			encoded, err := json.Marshal(block)
			if err == nil {
				texts = append(texts, string(encoded))
			}
		}
	}

	return strings.Join(texts, "\n\n")
}

func readBodySnippet(reader io.Reader) string {
	body, err := io.ReadAll(io.LimitReader(reader, 1024))
	if err != nil {
		return "read response body failed"
	}

	return string(body)
}
