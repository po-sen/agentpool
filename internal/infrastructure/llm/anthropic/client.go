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
	body, err := json.Marshal(messagesRequest{
		Model:     c.model,
		MaxTokens: 1024,
		System:    toAnthropicSystem(req.Messages),
		Messages:  toAnthropicMessages(req.Messages),
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
	for _, block := range decoded.Content {
		text := strings.TrimSpace(block.Text)
		if block.Type == "text" && text != "" {
			return outbound.ModelResponse{Content: text}, nil
		}
	}

	return outbound.ModelResponse{}, errors.New("anthropic response has no text content")
}

type messagesRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type messagesResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

func toAnthropicMessages(messages []outbound.ModelMessage) []anthropicMessage {
	items := make([]anthropicMessage, 0, len(messages))
	for _, message := range messages {
		if message.Role == "system" {
			continue
		}

		items = append(items, anthropicMessage{
			Role:    message.Role,
			Content: message.Content,
		})
	}

	return items
}

func toAnthropicSystem(messages []outbound.ModelMessage) string {
	var parts []string
	for _, message := range messages {
		if message.Role == "system" {
			parts = append(parts, message.Content)
		}
	}

	return strings.Join(parts, "\n\n")
}

func readBodySnippet(reader io.Reader) string {
	body, err := io.ReadAll(io.LimitReader(reader, 1024))
	if err != nil {
		return "read response body failed"
	}

	return string(body)
}
