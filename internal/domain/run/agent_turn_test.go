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
			Index:      1,
			Status:     AgentTurnStatusToolCall,
			ActionType: AgentTurnActionTypeToolCall,
			ToolName:   "workspace",
			Message:    "model requested tool call",
			RequestMessages: []AgentTurnMessage{
				{Role: "system", Content: "system prompt"},
				{Role: "tool", Content: "tool output", ToolCallID: "call_1", ToolName: "workspace"},
			},
			RawResponse:     `{"type":"tool_call"}`,
			ResponseFormat:  "json_object",
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
	if item.AgentTurns[0].ResponseFormat != "json_object" {
		t.Fatalf("ResponseFormat = %q, want json_object", item.AgentTurns[0].ResponseFormat)
	}
	if item.AgentTurns[0].RawResponse != `{"type":"tool_call"}` {
		t.Fatalf("RawResponse = %q, want raw tool call", item.AgentTurns[0].RawResponse)
	}
	if len(item.AgentTurns[0].RequestMessages) != 2 ||
		item.AgentTurns[0].RequestMessages[1].ToolCallID != "call_1" ||
		item.AgentTurns[0].RequestMessages[1].ToolName != "workspace" {
		t.Fatalf("RequestMessages = %#v, want copied model request messages", item.AgentTurns[0].RequestMessages)
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

func TestRunRecordAgentTurnsTruncatesRawDebugFields(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	item := newRun(t, now)

	item.RecordAgentTurns(now.Add(time.Second), []AgentTurn{
		{
			Index:       1,
			Status:      AgentTurnStatusProtocolError,
			RawResponse: strings.Repeat("r", MaxAgentTurnRawResponseLength+10),
			RequestMessages: []AgentTurnMessage{
				{Role: "user", Content: strings.Repeat("m", MaxAgentTurnMessageContentLength+10)},
			},
		},
	})

	if len(item.AgentTurns) != 1 {
		t.Fatalf("len(AgentTurns) = %d, want 1", len(item.AgentTurns))
	}
	if len(item.AgentTurns[0].RawResponse) != MaxAgentTurnRawResponseLength {
		t.Fatalf("len(RawResponse) = %d, want %d", len(item.AgentTurns[0].RawResponse), MaxAgentTurnRawResponseLength)
	}
	if !strings.HasSuffix(item.AgentTurns[0].RawResponse, toolCallResultTruncatedMarker) {
		t.Fatalf("RawResponse does not contain truncation marker: %q", item.AgentTurns[0].RawResponse)
	}
	if len(item.AgentTurns[0].RequestMessages[0].Content) != MaxAgentTurnMessageContentLength {
		t.Fatalf("len(RequestMessages[0].Content) = %d, want %d", len(item.AgentTurns[0].RequestMessages[0].Content), MaxAgentTurnMessageContentLength)
	}
}

func TestRunRecordAgentTurnsTruncatesCorrectionMessage(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	item := newRun(t, now)

	item.RecordAgentTurns(now.Add(time.Second), []AgentTurn{
		{
			Index:             1,
			Status:            AgentTurnStatusProtocolError,
			ProtocolErrorCode: "invalid_json",
			CorrectionMessage: strings.Repeat("b", MaxAgentTurnCorrectionLength+10),
		},
	})

	if len(item.AgentTurns) != 1 {
		t.Fatalf("len(AgentTurns) = %d, want 1", len(item.AgentTurns))
	}
	if item.AgentTurns[0].ProtocolErrorCode != "invalid_json" {
		t.Fatalf("ProtocolErrorCode = %q, want invalid_json", item.AgentTurns[0].ProtocolErrorCode)
	}
	if len(item.AgentTurns[0].CorrectionMessage) != MaxAgentTurnCorrectionLength {
		t.Fatalf("len(CorrectionMessage) = %d, want %d", len(item.AgentTurns[0].CorrectionMessage), MaxAgentTurnCorrectionLength)
	}
	if !strings.HasSuffix(item.AgentTurns[0].CorrectionMessage, toolCallResultTruncatedMarker) {
		t.Fatalf("CorrectionMessage does not contain truncation marker: %q", item.AgentTurns[0].CorrectionMessage)
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
			RequestMessages: []AgentTurnMessage{{Role: "user", Content: "do work"}},
			RawResponse:     "{bad}",
			ResponsePreview: "{bad}",
		},
	})

	clone := item.Clone()
	clone.AgentTurns[0].Status = AgentTurnStatusFinal
	clone.AgentTurns[0].RequestMessages[0].Content = "changed"
	clone.AgentTurns[0].RawResponse = "changed"
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
	if item.AgentTurns[0].RawResponse != "{bad}" {
		t.Fatalf("original raw response = %q, want {bad}", item.AgentTurns[0].RawResponse)
	}
	if item.AgentTurns[0].RequestMessages[0].Content != "do work" {
		t.Fatalf("original request message = %q, want do work", item.AgentTurns[0].RequestMessages[0].Content)
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
