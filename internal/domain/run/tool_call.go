package run

import (
	"strings"
	"time"
)

const (
	// MaxToolCallResultLength bounds stored tool output exposed through read APIs.
	MaxToolCallResultLength = 16 << 10

	toolCallResultTruncatedMarker = "\n... [truncated]"
)

// ToolCall records one provider-neutral tool execution observed during a run.
type ToolCall struct {
	Name      string
	Arguments map[string]string
	Result    string
	IsError   bool
	StartedAt time.Time
	EndedAt   time.Time
}

func copyToolCalls(calls []ToolCall) []ToolCall {
	if len(calls) == 0 {
		return nil
	}

	copied := make([]ToolCall, 0, len(calls))
	for _, call := range calls {
		if strings.TrimSpace(call.Name) == "" {
			continue
		}

		item := call
		item.Arguments = copyArguments(call.Arguments)
		item.Result = truncateToolCallResult(call.Result)
		copied = append(copied, item)
	}

	return copied
}

func copyArguments(arguments map[string]string) map[string]string {
	if len(arguments) == 0 {
		return nil
	}

	copied := make(map[string]string, len(arguments))
	for key, value := range arguments {
		copied[key] = value
	}

	return copied
}

func truncateToolCallResult(result string) string {
	if len(result) <= MaxToolCallResultLength {
		return result
	}

	maxContentLength := MaxToolCallResultLength - len(toolCallResultTruncatedMarker)
	if maxContentLength < 0 {
		maxContentLength = 0
	}

	return result[:maxContentLength] + toolCallResultTruncatedMarker
}
