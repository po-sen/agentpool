package run

import (
	"strings"
	"testing"
	"time"
)

func TestRunRecordToolCallsCopiesRecords(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	item := newRun(t, now)
	calls := []ToolCall{
		{
			Name: "read_file",
			Arguments: map[string]string{
				"path": "README.md",
			},
			Result:    "content",
			StartedAt: now.Add(time.Second),
			EndedAt:   now.Add(2 * time.Second),
		},
	}

	item.RecordToolCalls(now.Add(3*time.Second), calls)
	calls[0].Arguments["path"] = "changed.md"
	calls[0].Result = "changed"

	if len(item.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(item.ToolCalls))
	}
	if item.ToolCalls[0].Arguments["path"] != "README.md" {
		t.Fatalf("stored path = %q, want README.md", item.ToolCalls[0].Arguments["path"])
	}
	if item.ToolCalls[0].Result != "content" {
		t.Fatalf("stored result = %q, want content", item.ToolCalls[0].Result)
	}
	if !item.UpdatedAt.Equal(now.Add(3 * time.Second)) {
		t.Fatalf("UpdatedAt = %v, want %v", item.UpdatedAt, now.Add(3*time.Second))
	}
}

func TestRunRecordToolCallsReplacesRecordsAndSkipsMissingNames(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	item := newRun(t, now)
	item.RecordToolCalls(now, []ToolCall{{Name: "list_files", Result: "files"}})

	item.RecordToolCalls(now.Add(time.Second), []ToolCall{
		{Name: "   ", Result: "missing name"},
		{Name: "read_file", Result: "content"},
	})

	if len(item.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(item.ToolCalls))
	}
	if item.ToolCalls[0].Name != "read_file" {
		t.Fatalf("tool name = %q, want read_file", item.ToolCalls[0].Name)
	}
}

func TestRunCloneDeepCopiesToolCalls(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	item := newRun(t, now)
	item.RecordToolCalls(now.Add(time.Second), []ToolCall{
		{
			Name:      "run_shell",
			Arguments: map[string]string{"command": "pwd"},
			Result:    "exit_code: 0",
		},
	})

	clone := item.Clone()
	clone.ToolCalls[0].Arguments["command"] = "ls"
	clone.ToolCalls[0].Result = "changed"
	clone.ToolCalls = append(clone.ToolCalls, ToolCall{Name: "extra"})

	if len(item.ToolCalls) != 1 {
		t.Fatalf("len(original ToolCalls) = %d, want 1", len(item.ToolCalls))
	}
	if item.ToolCalls[0].Arguments["command"] != "pwd" {
		t.Fatalf("original command = %q, want pwd", item.ToolCalls[0].Arguments["command"])
	}
	if item.ToolCalls[0].Result != "exit_code: 0" {
		t.Fatalf("original result = %q, want exit_code: 0", item.ToolCalls[0].Result)
	}
}

func TestRunRecordToolCallsTruncatesResult(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	item := newRun(t, now)

	item.RecordToolCalls(now.Add(time.Second), []ToolCall{
		{Name: "read_file", Result: strings.Repeat("a", MaxToolCallResultLength+10)},
	})

	result := item.ToolCalls[0].Result
	if len(result) != MaxToolCallResultLength {
		t.Fatalf("len(result) = %d, want %d", len(result), MaxToolCallResultLength)
	}
	if !strings.HasSuffix(result, toolCallResultTruncatedMarker) {
		t.Fatalf("result does not contain truncation marker: %q", result)
	}
}
