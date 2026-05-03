package outbound_test

import (
	"context"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestModelClientContract(t *testing.T) {
	var client outbound.ModelClient = fakeModelClient{}

	response, err := client.Generate(context.Background(), outbound.ModelRequest{
		RunID: "run_test",
		Messages: []outbound.ModelMessage{
			{Role: "user", Content: "do work"},
		},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if response.Content == "" {
		t.Fatal("response.Content is empty")
	}
}

type fakeModelClient struct{}

func (fakeModelClient) Generate(context.Context, outbound.ModelRequest) (outbound.ModelResponse, error) {
	return outbound.ModelResponse{Content: "done"}, nil
}
