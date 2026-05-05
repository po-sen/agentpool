package agent

import (
	"strings"
	"testing"
)

func TestParseActionParsesFinalAction(t *testing.T) {
	result := parseAction(`{"type":"final","summary":"done"}`)
	assertValidAction(t, result, actionTypeFinal)
	if result.action.Summary != "done" {
		t.Fatalf("Summary = %q, want done", result.action.Summary)
	}
}

func TestParseActionParsesFinalBooleanAsString(t *testing.T) {
	result := parseAction(`{"type":"final","summary":true}`)
	assertValidAction(t, result, actionTypeFinal)
	if result.action.Summary != "true" {
		t.Fatalf("Summary = %q, want true", result.action.Summary)
	}
}

func TestParseActionParsesFinalNumberAsString(t *testing.T) {
	result := parseAction(`{"type":"final","summary":0.11}`)
	assertValidAction(t, result, actionTypeFinal)
	if result.action.Summary != "0.11" {
		t.Fatalf("Summary = %q, want 0.11", result.action.Summary)
	}
}

func TestParseActionParsesFencedFinalAction(t *testing.T) {
	result := parseAction("```JSON\n{\"type\":\"final\",\"summary\":\"done\"}\n```")
	assertValidAction(t, result, actionTypeFinal)
	if result.action.Summary != "done" {
		t.Fatalf("Summary = %q, want done", result.action.Summary)
	}
}

func TestParseActionExtractsOneEmbeddedJSONObject(t *testing.T) {
	result := parseAction("Here is the action:\n{\"type\":\"final\",\"summary\":\"done\"}\nThanks.")
	assertValidAction(t, result, actionTypeFinal)
	if result.action.Summary != "done" {
		t.Fatalf("Summary = %q, want done", result.action.Summary)
	}
}

func TestParseActionParsesToolCallAction(t *testing.T) {
	result := parseAction(`{"type":"tool_call","tool":"echo","arguments":{"text":"hello"}}`)
	assertValidAction(t, result, actionTypeToolCall)
	if result.action.Tool != "echo" {
		t.Fatalf("Tool = %q, want echo", result.action.Tool)
	}
	if result.action.Arguments["text"] != "hello" {
		t.Fatalf("text argument = %q, want hello", result.action.Arguments["text"])
	}
}

func TestParseActionParsesFencedToolCallAction(t *testing.T) {
	result := parseAction("```\n{\"type\":\"tool_call\",\"tool\":\"echo\",\"arguments\":{\"text\":\"hello\"}}\n```")
	assertValidAction(t, result, actionTypeToolCall)
	if result.action.Tool != "echo" {
		t.Fatalf("Tool = %q, want echo", result.action.Tool)
	}
}

func TestParseActionParsesToolCallScalarArgumentsAsStrings(t *testing.T) {
	result := parseAction(`{"type":"tool_call","tool":"sandbox_exec","arguments":{"timeout_seconds":30,"dry_run":false}}`)
	assertValidAction(t, result, actionTypeToolCall)
	if result.action.Arguments["timeout_seconds"] != "30" {
		t.Fatalf("timeout_seconds = %q, want 30", result.action.Arguments["timeout_seconds"])
	}
	if result.action.Arguments["dry_run"] != "false" {
		t.Fatalf("dry_run = %q, want false", result.action.Arguments["dry_run"])
	}
}

func TestParseActionRejectsInvalidJSONStringEscapeWithClearReason(t *testing.T) {
	result := parseAction(`{"type":"tool_call","tool":"sandbox_exec","arguments":{"command":"expr 123 \* 654321 \* 2"}}`)
	assertProtocolErrorCode(t, result, actionParseCodeInvalidJSON)
	for _, want := range []string{
		`JSON strings cannot use backslash before arbitrary punctuation such as \*`,
		`encode it as \\* in JSON`,
	} {
		if !strings.Contains(result.parseErr.Message+" "+result.parseErr.Hint, want) {
			t.Fatalf("parse error does not contain %q: %#v", want, result.parseErr)
		}
	}
}

func TestParseActionTreatsPlainTextAsNaturalLanguage(t *testing.T) {
	result := parseAction("done")
	if result.status != actionParseNaturalLanguage {
		t.Fatalf("status = %v, want %v", result.status, actionParseNaturalLanguage)
	}
}

func TestParseActionReturnsProtocolErrorForInvalidProtocol(t *testing.T) {
	tests := []struct {
		name string
		body string
		code string
	}{
		{
			name: "unknown action type",
			body: `{"type":"missing"}`,
			code: actionParseCodeUnknownActionType,
		},
		{
			name: "missing type",
			body: `{"summary":"done"}`,
			code: actionParseCodeMissingType,
		},
		{
			name: "empty final summary",
			body: `{"type":"final","summary":""}`,
			code: actionParseCodeInvalidSummary,
		},
		{
			name: "summary object",
			body: `{"type":"final","summary":{"value":"done"}}`,
			code: actionParseCodeInvalidSummary,
		},
		{
			name: "missing tool",
			body: `{"type":"tool_call","arguments":{"text":"hello"}}`,
			code: actionParseCodeMissingTool,
		},
		{
			name: "arguments array",
			body: `{"type":"tool_call","tool":"echo","arguments":[]}`,
			code: actionParseCodeInvalidArguments,
		},
		{
			name: "nested argument object",
			body: `{"type":"tool_call","tool":"echo","arguments":{"data":{"nested":true}}}`,
			code: actionParseCodeInvalidArgumentValue,
		},
		{
			name: "unknown field",
			body: `{"type":"final","summary":"done","foo":"bar"}`,
			code: actionParseCodeUnknownField,
		},
		{
			name: "multiple objects",
			body: `{"type":"tool_call","tool":"echo","arguments":{"text":"hello"}}{"type":"final","summary":"done"}`,
			code: actionParseCodeMultipleJSONValues,
		},
		{
			name: "multiple embedded objects",
			body: `First {"type":"tool_call","tool":"echo","arguments":{"text":"hello"}} then {"type":"final","summary":"done"}`,
			code: actionParseCodeMultipleJSONValues,
		},
		{
			name: "invalid json",
			body: `{"type":"final","summary":"done"`,
			code: actionParseCodeInvalidJSON,
		},
		{
			name: "non object action",
			body: `[{"type":"final","summary":"done"}]`,
			code: actionParseCodeNonObjectAction,
		},
		{
			name: "invalid expression json",
			body: `{"type":"final","summary":0.11 < 0.2}`,
			code: actionParseCodeInvalidJSON,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAction(tt.body)
			if tt.code == "" {
				if result.status != actionParseNaturalLanguage {
					t.Fatalf("status = %v, want natural language", result.status)
				}

				return
			}
			assertProtocolErrorCode(t, result, tt.code)
		})
	}
}

func assertValidAction(t *testing.T, result actionParseResult, want actionType) {
	t.Helper()

	if result.status != actionParseValid {
		t.Fatalf("status = %v, want %v; err = %v", result.status, actionParseValid, result.err)
	}
	if result.action.Type != want {
		t.Fatalf("Type = %q, want %q", result.action.Type, want)
	}
}

func assertProtocolErrorCode(t *testing.T, result actionParseResult, want string) {
	t.Helper()

	if result.status != actionParseProtocolError {
		t.Fatalf("status = %v, want %v", result.status, actionParseProtocolError)
	}
	if result.err == nil {
		t.Fatal("err = nil, want protocol error")
	}
	if result.parseErr.Code != want {
		t.Fatalf("parseErr.Code = %q, want %q; message=%q", result.parseErr.Code, want, result.parseErr.Message)
	}
	if result.parseErr.Message == "" {
		t.Fatal("parseErr.Message is empty")
	}
	if result.parseErr.Hint == "" {
		t.Fatal("parseErr.Hint is empty")
	}
}
