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
	body, err := json.Marshal(generateContentRequest{
		Contents: toGeminiContents(req.Messages),
	})
	if err != nil {
		return outbound.ModelResponse{}, err
	}

	endpoint := fmt.Sprintf("%s/models/%s:generateContent", c.baseURL, url.PathEscape(c.model))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return outbound.ModelResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", c.apiKey)

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return outbound.ModelResponse{}, err
	}
	defer func() {
		_ = httpResp.Body.Close()
	}()

	if httpResp.StatusCode < http.StatusOK || httpResp.StatusCode >= http.StatusMultipleChoices {
		return outbound.ModelResponse{}, fmt.Errorf("gemini request failed: status %d: %s", httpResp.StatusCode, readBodySnippet(httpResp.Body))
	}

	var decoded generateContentResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&decoded); err != nil {
		return outbound.ModelResponse{}, fmt.Errorf("decode gemini response: %w", err)
	}
	for _, candidate := range decoded.Candidates {
		for _, part := range candidate.Content.Parts {
			text := strings.TrimSpace(part.Text)
			if text != "" {
				return outbound.ModelResponse{Content: text}, nil
			}
		}
	}

	return outbound.ModelResponse{}, errors.New("gemini response has no text content")
}

type generateContentRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type generateContentResponse struct {
	Candidates []struct {
		Content struct {
			Parts []geminiPart `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

func toGeminiContents(messages []outbound.ModelMessage) []geminiContent {
	items := make([]geminiContent, 0, len(messages))
	for _, message := range messages {
		items = append(items, geminiContent{
			Role:  message.Role,
			Parts: []geminiPart{{Text: message.Content}},
		})
	}

	return items
}

func readBodySnippet(reader io.Reader) string {
	body, err := io.ReadAll(io.LimitReader(reader, 1024))
	if err != nil {
		return "read response body failed"
	}

	return string(body)
}
