package noop

import (
	"context"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

// Client returns placeholder model responses.
type Client struct{}

var _ outbound.ModelClient = (*Client)(nil)

// NewClient creates a no-op model client.
func NewClient() *Client {
	return &Client{}
}

// Generate returns a placeholder model response.
func (c *Client) Generate(context.Context, outbound.ModelRequest) (outbound.ModelResponse, error) {
	return outbound.ModelResponse{Content: "noop model response"}, nil
}
