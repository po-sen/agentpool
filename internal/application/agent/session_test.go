package agent

import (
	"testing"
	"time"
)

func TestRunSessionRecordToolCallCopiesArguments(t *testing.T) {
	session := &runSession{}
	arguments := map[string]string{"path": "/workspace/a.txt"}

	session.recordToolCall("workspace", arguments, "ok", false, time.Unix(1, 0), time.Unix(2, 0))
	arguments["path"] = "changed"

	if len(session.toolCalls) != 1 {
		t.Fatalf("len(toolCalls) = %d, want 1", len(session.toolCalls))
	}
	if session.toolCalls[0].Arguments["path"] != "/workspace/a.txt" {
		t.Fatalf("recorded path = %q, want copied argument", session.toolCalls[0].Arguments["path"])
	}
}

func TestRunSessionTracksPendingToolErrorRecovery(t *testing.T) {
	session := &runSession{availableToolNames: []string{"sandbox_exec"}}

	session.recordToolCall("sandbox_exec", nil, "failed", true, time.Unix(1, 0), time.Unix(2, 0))
	if !session.shouldRejectFinalAfterToolError() {
		t.Fatal("shouldRejectFinalAfterToolError() = false, want true after tool error")
	}

	session.recordToolCall("sandbox_exec", nil, "ok", false, time.Unix(3, 0), time.Unix(4, 0))
	if session.shouldRejectFinalAfterToolError() {
		t.Fatal("shouldRejectFinalAfterToolError() = true, want false after successful tool call")
	}
}

func TestRunSessionTracksUsedToolCallSignatures(t *testing.T) {
	session := &runSession{}

	session.recordToolCallSignature("sandbox_exec", map[string]string{
		"max_output_bytes": "65536",
		"command":          "python3 -c 'print(42)'",
	})

	if !session.toolCallWasUsed("sandbox_exec", map[string]string{
		"command":          "python3 -c 'print(42)'",
		"max_output_bytes": "65536",
	}) {
		t.Fatal("toolCallWasUsed() = false, want true for same arguments in different map order")
	}
	if !session.toolCallWasUsed(" sandbox_exec ", map[string]string{
		"command":            " python3 -c 'print(42)' ",
		" max_output_bytes ": "65536",
	}) {
		t.Fatal("toolCallWasUsed() = false, want true for trimmed tool and argument values")
	}
	if session.toolCallWasUsed("sandbox_exec", map[string]string{
		"command":          "python3 -c 'print(43)'",
		"max_output_bytes": "65536",
	}) {
		t.Fatal("toolCallWasUsed() = true, want false for changed arguments")
	}
}
