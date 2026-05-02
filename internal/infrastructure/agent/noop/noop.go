package noop

import (
	"context"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

// Executor returns a placeholder execution result.
type Executor struct{}

var _ outbound.AgentExecutor = (*Executor)(nil)

// NewExecutor creates a no-op agent executor.
func NewExecutor() *Executor {
	return &Executor{}
}

// Execute returns a placeholder summary.
func (e *Executor) Execute(context.Context, outbound.AgentExecutionRequest) (outbound.AgentExecutionResult, error) {
	return outbound.AgentExecutionResult{Summary: "noop agent executor"}, nil
}
