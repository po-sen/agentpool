package agent

import "testing"

func TestParseActionParsesFinalAction(t *testing.T) {
	parsed, ok := parseAction(`{"type":"final","summary":"done"}`)
	if !ok {
		t.Fatal("parseAction() ok = false, want true")
	}
	if parsed.Type != actionTypeFinal {
		t.Fatalf("Type = %q, want %q", parsed.Type, actionTypeFinal)
	}
	if parsed.Summary != "done" {
		t.Fatalf("Summary = %q, want done", parsed.Summary)
	}
}

func TestParseActionParsesToolCallAction(t *testing.T) {
	parsed, ok := parseAction(`{"type":"tool_call","tool":"echo","arguments":{"text":"hello"}}`)
	if !ok {
		t.Fatal("parseAction() ok = false, want true")
	}
	if parsed.Type != actionTypeToolCall {
		t.Fatalf("Type = %q, want %q", parsed.Type, actionTypeToolCall)
	}
	if parsed.Tool != "echo" {
		t.Fatalf("Tool = %q, want echo", parsed.Tool)
	}
	if parsed.Arguments["text"] != "hello" {
		t.Fatalf("text argument = %q, want hello", parsed.Arguments["text"])
	}
}

func TestParseActionRejectsInvalidContent(t *testing.T) {
	tests := []string{
		"plain text",
		`{"type":"missing"}`,
		`{"summary":"done"}`,
		`{"type":"tool_call","arguments":[]}`,
	}

	for _, tt := range tests {
		t.Run(tt, func(t *testing.T) {
			if _, ok := parseAction(tt); ok {
				t.Fatal("parseAction() ok = true, want false")
			}
		})
	}
}
