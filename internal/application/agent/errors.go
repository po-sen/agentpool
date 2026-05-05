package agent

// ErrorCode is a provider-neutral, safe machine-readable agent failure code.
type ErrorCode string

const (
	// ErrorCodeUnknown identifies an unclassified agent failure.
	ErrorCodeUnknown ErrorCode = "unknown_agent_error"
	// ErrorCodeModelGenerateFailed identifies a model generation failure.
	ErrorCodeModelGenerateFailed ErrorCode = "model_generate_failed"
	// ErrorCodeToolListFailed identifies a tool discovery failure.
	ErrorCodeToolListFailed ErrorCode = "tool_list_failed"
	// ErrorCodeToolExecutionFailed identifies a tool execution failure.
	ErrorCodeToolExecutionFailed ErrorCode = "tool_execution_failed"
	// ErrorCodeAgentMaxTurns identifies an agent loop max-turns failure.
	ErrorCodeAgentMaxTurns ErrorCode = "agent_max_turns"
	// ErrorCodeFinalSummaryInvalid identifies an invalid final summary failure.
	ErrorCodeFinalSummaryInvalid ErrorCode = "final_summary_invalid"
	// ErrorCodeAgentProtocolError identifies an unrecoverable agent protocol failure.
	ErrorCodeAgentProtocolError ErrorCode = "agent_protocol_error"
)

const (
	messageUnknownAgentError   = "agent failed"
	messageModelGenerateFailed = "model generation failed"
	messageToolListFailed      = "tool list failed"
	messageToolExecutionFailed = "tool execution failed"
	messageAgentMaxTurns       = "agent reached max turns"
	messageFinalSummaryInvalid = "agent final summary is invalid"
	messageAgentProtocolError  = "agent protocol error"
)

// Error wraps an internal cause with a safe code and message for run diagnostics.
type Error struct {
	Code    ErrorCode
	Message string
	Cause   error
}

// Error returns only the safe diagnostic message.
func (e Error) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Code != "" {
		return string(e.Code)
	}

	return messageUnknownAgentError
}

// Unwrap returns the internal cause for application code and tests.
func (e Error) Unwrap() error {
	return e.Cause
}

func newAgentError(code ErrorCode, message string, cause error) Error {
	if code == "" {
		code = ErrorCodeUnknown
	}
	if message == "" {
		message = defaultAgentErrorMessage(code)
	}

	return Error{Code: code, Message: message, Cause: cause}
}

func defaultAgentErrorMessage(code ErrorCode) string {
	switch code {
	case ErrorCodeModelGenerateFailed:
		return messageModelGenerateFailed
	case ErrorCodeToolListFailed:
		return messageToolListFailed
	case ErrorCodeToolExecutionFailed:
		return messageToolExecutionFailed
	case ErrorCodeAgentMaxTurns:
		return messageAgentMaxTurns
	case ErrorCodeFinalSummaryInvalid:
		return messageFinalSummaryInvalid
	case ErrorCodeAgentProtocolError:
		return messageAgentProtocolError
	default:
		return messageUnknownAgentError
	}
}
