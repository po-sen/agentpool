package outbound

import (
	"context"

	"github.com/po-sen/agentpool/internal/domain/run"
)

// ModelMessage is a provider-neutral model conversation message.
type ModelMessage struct {
	Role    string
	Content string
}

// ModelRequest describes a provider-neutral model generation request.
type ModelRequest struct {
	RunID    run.RunID
	Messages []ModelMessage
}

// ModelResponse contains generated model content.
type ModelResponse struct {
	Content string
}

// ModelClient generates content from provider-neutral model requests.
type ModelClient interface {
	Generate(context.Context, ModelRequest) (ModelResponse, error)
}
