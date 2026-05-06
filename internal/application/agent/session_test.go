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
