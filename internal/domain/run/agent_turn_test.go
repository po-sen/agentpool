package run

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestRunRecordAgentTurnsCopiesAndTruncatesRecords(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	item := newRun(t, now)
	turns := []AgentTurn{
		{
			Index:           1,
			Status:          AgentTurnStatusToolCall,
			ActionType:      AgentTurnActionTypeToolCall,
			ToolName:        "read_file",
			Message:         "model requested tool call",
			ResponsePreview: strings.Repeat("a", MaxAgentTurnPreviewLength+10),
			StartedAt:       now.Add(time.Second),
			EndedAt:         now.Add(2 * time.Second),
		},
	}

	item.RecordAgentTurns(now.Add(3*time.Second), turns)
	turns[0].Status = AgentTurnStatusFinal
	turns[0].ResponsePreview = "changed"

	if len(item.AgentTurns) != 1 {
		t.Fatalf("len(AgentTurns) = %d, want 1", len(item.AgentTurns))
	}
	if item.AgentTurns[0].Status != AgentTurnStatusToolCall {
		t.Fatalf("stored status = %q, want %q", item.AgentTurns[0].Status, AgentTurnStatusToolCall)
	}
	if len(item.AgentTurns[0].ResponsePreview) != MaxAgentTurnPreviewLength {
		t.Fatalf("len(ResponsePreview) = %d, want %d", len(item.AgentTurns[0].ResponsePreview), MaxAgentTurnPreviewLength)
	}
	if !strings.HasSuffix(item.AgentTurns[0].ResponsePreview, toolCallResultTruncatedMarker) {
		t.Fatalf("ResponsePreview does not contain truncation marker: %q", item.AgentTurns[0].ResponsePreview)
	}
	if !item.UpdatedAt.Equal(now.Add(3 * time.Second)) {
		t.Fatalf("UpdatedAt = %v, want %v", item.UpdatedAt, now.Add(3*time.Second))
	}
}

func TestRunRecordAgentTurnsSkipsInvalidIndexes(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	item := newRun(t, now)

	item.RecordAgentTurns(now.Add(time.Second), []AgentTurn{
		{Index: 0, Status: AgentTurnStatusToolCall},
		{Index: -1, Status: AgentTurnStatusFinal},
		{Index: 1, Status: AgentTurnStatusMaxTurns},
	})

	if len(item.AgentTurns) != 1 {
		t.Fatalf("len(AgentTurns) = %d, want 1", len(item.AgentTurns))
	}
	if item.AgentTurns[0].Status != AgentTurnStatusMaxTurns {
		t.Fatalf("status = %q, want %q", item.AgentTurns[0].Status, AgentTurnStatusMaxTurns)
	}
}

func TestRunCloneDeepCopiesAgentTurns(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	item := newRun(t, now)
	item.RecordAgentTurns(now.Add(time.Second), []AgentTurn{
		{
			Index:           1,
			Status:          AgentTurnStatusProtocolError,
			Message:         "model response did not match AgentPool action protocol",
			ResponsePreview: "{bad}",
		},
	})

	clone := item.Clone()
	clone.AgentTurns[0].Status = AgentTurnStatusFinal
	clone.AgentTurns[0].ResponsePreview = "changed"
	clone.AgentTurns = append(clone.AgentTurns, AgentTurn{Index: 2, Status: AgentTurnStatusMaxTurns})

	if len(item.AgentTurns) != 1 {
		t.Fatalf("len(original AgentTurns) = %d, want 1", len(item.AgentTurns))
	}
	if item.AgentTurns[0].Status != AgentTurnStatusProtocolError {
		t.Fatalf("original status = %q, want %q", item.AgentTurns[0].Status, AgentTurnStatusProtocolError)
	}
	if item.AgentTurns[0].ResponsePreview != "{bad}" {
		t.Fatalf("original preview = %q, want {bad}", item.AgentTurns[0].ResponsePreview)
	}
}

func TestRunRecordAgentTurnsTruncatesPreviewWithoutBreakingUTF8(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	item := newRun(t, now)

	item.RecordAgentTurns(now.Add(time.Second), []AgentTurn{
		{Index: 1, Status: AgentTurnStatusProtocolError, ResponsePreview: strings.Repeat("界", MaxAgentTurnPreviewLength)},
	})

	preview := item.AgentTurns[0].ResponsePreview
	if len(preview) > MaxAgentTurnPreviewLength {
		t.Fatalf("len(ResponsePreview) = %d, want <= %d", len(preview), MaxAgentTurnPreviewLength)
	}
	if !utf8.ValidString(preview) {
		t.Fatalf("ResponsePreview is not valid UTF-8: %q", preview)
	}
	if !strings.HasSuffix(preview, toolCallResultTruncatedMarker) {
		t.Fatalf("ResponsePreview does not contain truncation marker: %q", preview)
	}
}
