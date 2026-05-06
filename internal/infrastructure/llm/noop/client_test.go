package noop

import (
	"context"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestClientGenerateReturnsContent(t *testing.T) {
	var client outbound.ModelClient = NewClient()

	response, err := client.Generate(context.Background(), outbound.ModelRequest{
		RunID: "run_test",
		Turns: []outbound.ModelTurn{
			{
				Role: outbound.ModelRoleUser,
				Parts: []outbound.ModelPart{
					{Kind: outbound.ModelPartKindTaskPrompt, Text: "do work"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if response.Content == "" {
		t.Fatal("response.Content is empty")
	}
}
