package outbound

import (
	"context"
	"testing"
)

func TestModelClientContract(t *testing.T) {
	var client ModelClient = fakeModelClient{}

	response, err := client.Generate(context.Background(), ModelRequest{
		RunID:        "run_test",
		Instructions: "follow protocol",
		Turns: []ModelTurn{
			{
				Role: ModelRoleUser,
				Parts: []ModelPart{
					{Kind: ModelPartKindTaskPrompt, Text: "do work"},
				},
			},
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

func (fakeModelClient) Generate(context.Context, ModelRequest) (ModelResponse, error) {
	return ModelResponse{Content: "done"}, nil
}
