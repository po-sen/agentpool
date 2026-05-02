package noop_test

import (
	"context"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	agentnoop "github.com/po-sen/agentpool/internal/infrastructure/agent/noop"
)

func TestExecutorExecuteReturnsPlaceholderSummary(t *testing.T) {
	executor := agentnoop.NewExecutor()

	result, err := executor.Execute(context.Background(), outbound.AgentExecutionRequest{})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Summary == "" {
		t.Fatal("summary is empty")
	}
}
