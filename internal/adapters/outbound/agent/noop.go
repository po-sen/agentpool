package agent

import (
	"context"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

// NoopExecutor returns a placeholder execution result.
type NoopExecutor struct{}

var _ outbound.AgentExecutor = (*NoopExecutor)(nil)

// NewNoopExecutor creates a no-op agent executor.
func NewNoopExecutor() *NoopExecutor {
	return &NoopExecutor{}
}

// Execute returns a placeholder summary.
func (e *NoopExecutor) Execute(context.Context, outbound.AgentExecutionRequest) (outbound.AgentExecutionResult, error) {
	return outbound.AgentExecutionResult{Summary: "noop agent executor"}, nil
}
