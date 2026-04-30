package agent_test

import (
	"context"
	"testing"

	"github.com/po-sen/agentpool/internal/adapters/outbound/agent"
	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestNoopExecutorExecuteReturnsPlaceholderSummary(t *testing.T) {
	executor := agent.NewNoopExecutor()

	result, err := executor.Execute(context.Background(), outbound.AgentExecutionRequest{})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Summary == "" {
		t.Fatal("summary is empty")
	}
}
