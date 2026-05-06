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
func (c *Client) Generate(_ context.Context, request outbound.ModelRequest) (outbound.ModelResponse, error) {
	return outbound.ModelResponse{
		Content:         "noop model response",
		RequestMessages: toModelRequestMessages(request),
	}, nil
}

func toModelRequestMessages(request outbound.ModelRequest) []outbound.ModelRequestMessage {
	messages := []outbound.ModelRequestMessage{}
	if request.Instructions != "" {
		messages = append(messages, outbound.ModelRequestMessage{
			Role:    "system",
			Content: request.Instructions,
		})
	}
	for _, turn := range request.Turns {
		for _, part := range turn.Parts {
			messages = append(messages, outbound.ModelRequestMessage{
				Role:       toModelRequestRole(turn.Role),
				Content:    part.Text,
				ToolCallID: part.ToolCallID,
				ToolName:   part.ToolName,
			})
		}
	}

	return messages
}

func toModelRequestRole(role outbound.ModelRole) string {
	if role == outbound.ModelRoleRuntime {
		return "user"
	}

	return string(role)
}
