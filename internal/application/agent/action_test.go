package agent

import "testing"

func TestParseActionParsesFinalAction(t *testing.T) {
	result := parseAction(`{"type":"final","summary":"done"}`)
	if result.status != actionParseValid {
		t.Fatalf("status = %v, want %v; err = %v", result.status, actionParseValid, result.err)
	}
	if result.action.Type != actionTypeFinal {
		t.Fatalf("Type = %q, want %q", result.action.Type, actionTypeFinal)
	}
	if result.action.Summary != "done" {
		t.Fatalf("Summary = %q, want done", result.action.Summary)
	}
}

func TestParseActionParsesToolCallAction(t *testing.T) {
	result := parseAction(`{"type":"tool_call","tool":"echo","arguments":{"text":"hello"}}`)
	if result.status != actionParseValid {
		t.Fatalf("status = %v, want %v; err = %v", result.status, actionParseValid, result.err)
	}
	if result.action.Type != actionTypeToolCall {
		t.Fatalf("Type = %q, want %q", result.action.Type, actionTypeToolCall)
	}
	if result.action.Tool != "echo" {
		t.Fatalf("Tool = %q, want echo", result.action.Tool)
	}
	if result.action.Arguments["text"] != "hello" {
		t.Fatalf("text argument = %q, want hello", result.action.Arguments["text"])
	}
}

func TestParseActionTreatsPlainTextAsNaturalLanguage(t *testing.T) {
	result := parseAction("done")
	if result.status != actionParseNaturalLanguage {
		t.Fatalf("status = %v, want %v", result.status, actionParseNaturalLanguage)
	}
}

func TestParseActionReturnsProtocolErrorForInvalidJSONProtocol(t *testing.T) {
	tests := []string{
		`{"type":"missing"}`,
		`{"summary":"done"}`,
		`{"type":"final","summary":""}`,
		`{"type":"tool_call","arguments":{"text":"hello"}}`,
		`{"type":"tool_call","tool":"echo","arguments":[]}`,
		`{"type":"tool_result","result":"hello from tool"}`,
		`{"type":"tool_call","tool":"echo","arguments":{"text":"hello"}}{"type":"final","summary":"done"}`,
		`{"type":"final","summary":"done"`,
		`[{"type":"final","summary":"done"}]`,
		"```json\n{\"type\":\"tool_call\",\"tool\":\"echo\",\"arguments\":{\"text\":\"hello\"}}\n```",
	}

	for _, tt := range tests {
		t.Run(tt, func(t *testing.T) {
			result := parseAction(tt)
			if result.status != actionParseProtocolError {
				t.Fatalf("status = %v, want %v", result.status, actionParseProtocolError)
			}
			if result.err == nil {
				t.Fatal("err = nil, want protocol error")
			}
		})
	}
}
