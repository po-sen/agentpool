package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

const defaultTimeout = 30 * time.Second

// Config contains Gemini model client configuration.
type Config struct {
	BaseURL string
	Model   string
	APIKey  string
	Timeout time.Duration
}

// Client calls the Gemini generateContent API.
type Client struct {
	httpClient *http.Client
	baseURL    string
	model      string
	apiKey     string
}

var _ outbound.ModelClient = (*Client)(nil)

// NewClient creates a Gemini model client.
func NewClient(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		return nil, errors.New("gemini base URL is required")
	}
	if cfg.Model == "" {
		return nil, errors.New("gemini model is required")
	}
	if cfg.APIKey == "" {
		return nil, errors.New("gemini API key is required")
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

// Generate sends a generateContent request and returns generated content.
func (c *Client) Generate(ctx context.Context, req outbound.ModelRequest) (outbound.ModelResponse, error) {
	requestMessages := toModelRequestMessages(req)
	request := toGenerateContentRequest(req)
	body, err := json.Marshal(request)
	if err != nil {
		return outbound.ModelResponse{RequestMessages: requestMessages}, err
	}

	endpoint := fmt.Sprintf("%s/models/%s:generateContent", c.baseURL, url.PathEscape(c.model))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return outbound.ModelResponse{RequestMessages: requestMessages}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", c.apiKey)

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return outbound.ModelResponse{RequestMessages: requestMessages}, err
	}
	defer func() {
		_ = httpResp.Body.Close()
	}()

	if httpResp.StatusCode < http.StatusOK || httpResp.StatusCode >= http.StatusMultipleChoices {
		return outbound.ModelResponse{RequestMessages: requestMessages},
			fmt.Errorf("gemini request failed: status %d: %s", httpResp.StatusCode, readBodySnippet(httpResp.Body))
	}

	var decoded generateContentResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&decoded); err != nil {
		return outbound.ModelResponse{RequestMessages: requestMessages}, fmt.Errorf("decode gemini response: %w", err)
	}
	for _, candidate := range decoded.Candidates {
		texts := []string{}
		toolCalls := []outbound.ModelToolCall{}
		for _, part := range candidate.Content.Parts {
			text := strings.TrimSpace(part.Text)
			if text != "" {
				texts = append(texts, text)
			}
			if part.FunctionCall != nil {
				name := strings.TrimSpace(part.FunctionCall.Name)
				if name == "" {
					continue
				}
				toolCalls = append(toolCalls, outbound.ModelToolCall{
					ID:        strings.TrimSpace(part.FunctionCall.ID),
					Name:      name,
					Arguments: stringifyFunctionArgs(part.FunctionCall.Args),
				})
			}
		}
		if len(texts) > 0 || len(toolCalls) > 0 {
			return outbound.ModelResponse{
				Content:         strings.Join(texts, "\n\n"),
				ToolCalls:       toolCalls,
				RequestMessages: requestMessages,
			}, nil
		}
	}

	return outbound.ModelResponse{RequestMessages: requestMessages}, errors.New("gemini response has no text content")
}

type generateContentRequest struct {
	SystemInstruction *geminiSystemInstruction `json:"system_instruction,omitempty"`
	Contents          []geminiContent          `json:"contents"`
	Tools             []geminiTool             `json:"tools,omitempty"`
}

type geminiSystemInstruction struct {
	Parts []geminiPart `json:"parts"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text             string                  `json:"text,omitempty"`
	FunctionCall     *geminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse `json:"functionResponse,omitempty"`
}

type generateContentResponse struct {
	Candidates []struct {
		Content struct {
			Parts []geminiPart `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

type geminiFunctionCall struct {
	ID   string                 `json:"id,omitempty"`
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args,omitempty"`
}

type geminiFunctionResponse struct {
	ID       string                 `json:"id,omitempty"`
	Name     string                 `json:"name"`
	Response map[string]interface{} `json:"response"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFunctionDeclaration `json:"functionDeclarations"`
}

type geminiFunctionDeclaration struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Parameters  geminiParameters `json:"parameters"`
}

type geminiParameters struct {
	Type                 string                           `json:"type"`
	Properties           map[string]geminiParameterSchema `json:"properties"`
	Required             []string                         `json:"required,omitempty"`
	AdditionalProperties bool                             `json:"additionalProperties"`
}

type geminiParameterSchema struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

func toGeminiContents(turns []outbound.ModelTurn) []geminiContent {
	items := make([]geminiContent, 0, len(turns))
	for _, turn := range turns {
		if turn.Role == outbound.ModelRoleRuntime || isAssistantAttemptTurn(turn) {
			continue
		}
		parts := toGeminiParts(turn.Parts)
		if len(parts) == 0 {
			continue
		}

		items = appendOrMergeGeminiContent(items, geminiContent{
			Role:  toGeminiRole(turn.Role),
			Parts: parts,
		})
	}

	return items
}

func toGenerateContentRequest(modelRequest outbound.ModelRequest) generateContentRequest {
	request := generateContentRequest{
		Contents: toGeminiContents(modelRequest.Turns),
		Tools:    toGeminiTools(modelRequest.Tools),
	}

	if parts := toGeminiSystemParts(modelRequest); len(parts) > 0 {
		request.SystemInstruction = &geminiSystemInstruction{
			Parts: parts,
		}
	}

	return request
}

func toModelRequestMessages(modelRequest outbound.ModelRequest) []outbound.ModelRequestMessage {
	items := []outbound.ModelRequestMessage{}
	if content := joinGeminiSystemDiagnostics(modelRequest); content != "" {
		items = append(items, outbound.ModelRequestMessage{
			Role:    "system_instruction",
			Content: content,
		})
	}
	for _, content := range toGeminiContents(modelRequest.Turns) {
		items = append(items, outbound.ModelRequestMessage{
			Role:    content.Role,
			Content: geminiPartsDiagnostic(content.Parts),
		})
	}

	return items
}

func joinGeminiSystemDiagnostics(modelRequest outbound.ModelRequest) string {
	parts := []string{}
	if instructions := strings.TrimSpace(modelRequest.Instructions); instructions != "" {
		parts = append(parts, instructions)
	}
	for _, turn := range modelRequest.Turns {
		switch {
		case turn.Role == outbound.ModelRoleRuntime:
			if content := joinGeminiPartTexts(toGeminiParts(turn.Parts)); content != "" {
				parts = append(parts, content)
			}
		case isAssistantAttemptTurn(turn):
			if text := joinSystemAttemptParts(turn.Parts); text != "" {
				parts = append(parts, text)
			}
		}
	}

	return strings.Join(parts, "\n\n")
}

func joinGeminiPartTexts(parts []geminiPart) string {
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		text := strings.TrimSpace(part.Text)
		if text != "" {
			texts = append(texts, text)
		}
	}

	return strings.Join(texts, "\n\n")
}

func toGeminiSystemParts(modelRequest outbound.ModelRequest) []geminiPart {
	parts := []geminiPart{}
	if system := strings.TrimSpace(modelRequest.Instructions); system != "" {
		parts = append(parts, geminiPart{Text: system})
	}
	for _, turn := range modelRequest.Turns {
		switch {
		case turn.Role == outbound.ModelRoleRuntime:
			parts = append(parts, toGeminiParts(turn.Parts)...)
		case isAssistantAttemptTurn(turn):
			if text := joinSystemAttemptParts(turn.Parts); text != "" {
				parts = append(parts, geminiPart{Text: text})
			}
		}
	}

	return parts
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

func joinSystemAttemptParts(parts []outbound.ModelPart) string {
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		text := strings.TrimSpace(part.Text)
		if text != "" {
			texts = append(texts, text)
		}
	}
	if len(texts) == 0 {
		return ""
	}

	return "Previous assistant attempt that failed validation:\n" + strings.Join(texts, "\n\n")
}

func toGeminiParts(parts []outbound.ModelPart) []geminiPart {
	items := make([]geminiPart, 0, len(parts))
	for _, part := range parts {
		item, ok := toGeminiPart(part)
		if ok {
			items = append(items, item)
		}
	}

	return items
}

func toGeminiPart(part outbound.ModelPart) (geminiPart, bool) {
	switch part.Kind {
	case outbound.ModelPartKindToolCall:
		return toGeminiFunctionCallPart(part)
	case outbound.ModelPartKindToolResult:
		return toGeminiFunctionResponsePart(part)
	default:
		return toGeminiTextPart(part)
	}
}

func toGeminiFunctionCallPart(part outbound.ModelPart) (geminiPart, bool) {
	name := strings.TrimSpace(part.ToolName)
	if name == "" {
		return geminiPart{}, false
	}

	return geminiPart{
		FunctionCall: &geminiFunctionCall{
			ID:   strings.TrimSpace(part.ToolCallID),
			Name: name,
			Args: stringMapToInterfaceMap(part.ToolArguments),
		},
	}, true
}

func toGeminiFunctionResponsePart(part outbound.ModelPart) (geminiPart, bool) {
	name := strings.TrimSpace(part.ToolName)
	content := strings.TrimSpace(part.Text)
	if name == "" || content == "" {
		return geminiPart{}, false
	}

	return geminiPart{
		FunctionResponse: &geminiFunctionResponse{
			ID:   strings.TrimSpace(part.ToolCallID),
			Name: name,
			Response: map[string]interface{}{
				"result":   content,
				"is_error": part.IsError,
			},
		},
	}, true
}

func toGeminiTextPart(part outbound.ModelPart) (geminiPart, bool) {
	text := strings.TrimSpace(part.Text)
	if text == "" {
		return geminiPart{}, false
	}
	if part.Kind == outbound.ModelPartKindWorkspaceContext {
		text = "Workspace context:\n" + text
	}

	return geminiPart{Text: text}, true
}

func appendOrMergeGeminiContent(items []geminiContent, item geminiContent) []geminiContent {
	if len(items) == 0 || items[len(items)-1].Role != item.Role {
		return append(items, item)
	}

	items[len(items)-1].Parts = append(items[len(items)-1].Parts, item.Parts...)

	return items
}

func toGeminiRole(role outbound.ModelRole) string {
	if role == outbound.ModelRoleAssistant {
		return "model"
	}

	return "user"
}

func toGeminiTools(tools []outbound.ToolDefinition) []geminiTool {
	if len(tools) == 0 {
		return nil
	}

	declarations := make([]geminiFunctionDeclaration, 0, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		declarations = append(declarations, geminiFunctionDeclaration{
			Name:        name,
			Description: strings.TrimSpace(tool.Description),
			Parameters:  toGeminiParameters(tool.Arguments),
		})
	}
	if len(declarations) == 0 {
		return nil
	}

	return []geminiTool{{FunctionDeclarations: declarations}}
}

func toGeminiParameters(arguments []outbound.ToolArgumentDefinition) geminiParameters {
	properties := map[string]geminiParameterSchema{}
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
		properties[name] = geminiParameterSchema{
			Type:        "string",
			Description: description,
		}
		if argument.Required {
			required = append(required, name)
		}
	}

	return geminiParameters{
		Type:                 "object",
		Properties:           properties,
		Required:             required,
		AdditionalProperties: false,
	}
}

func stringifyFunctionArgs(args map[string]interface{}) map[string]string {
	if len(args) == 0 {
		return nil
	}

	items := make(map[string]string, len(args))
	for key, value := range args {
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

func stringMapToInterfaceMap(values map[string]string) map[string]interface{} {
	if len(values) == 0 {
		return nil
	}

	items := make(map[string]interface{}, len(values))
	for key, value := range values {
		items[key] = value
	}

	return items
}

func geminiPartsDiagnostic(parts []geminiPart) string {
	if len(parts) == 0 {
		return ""
	}
	texts := []string{}
	for _, part := range parts {
		if text := strings.TrimSpace(part.Text); text != "" {
			texts = append(texts, text)
			continue
		}
		encoded, err := json.Marshal(part)
		if err == nil {
			texts = append(texts, string(encoded))
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
