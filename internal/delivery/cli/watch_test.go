package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestWriteRunTimelineUpdateIncludesTurnAndToolDetails(t *testing.T) {
	startedAt := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	endedAt := startedAt.Add(time.Second)
	response := RunResponse{
		ID:        "run_timeline",
		Status:    statusCompleted,
		UpdatedAt: endedAt,
		Result:    &RunResultResponse{Summary: "done"},
		AgentTurns: []AgentTurnResponse{
			{
				Index:       2,
				Status:      "tool_call",
				ActionType:  "tool_call",
				ToolName:    "sandbox_exec",
				Message:     "model requested tool call",
				RawResponse: `{"type":"tool_call","tool":"sandbox_exec","arguments":{"command":"wc -l /workspace/README.md","timeout_seconds":"10"}}`,
				StartedAt:   startedAt,
				EndedAt:     endedAt,
			},
		},
		ToolCalls: []ToolCallResponse{
			{
				Name:      "sandbox_exec",
				Arguments: map[string]string{"command": "wc -l /workspace/README.md"},
				Result:    "12 /workspace/README.md",
				StartedAt: startedAt,
				EndedAt:   endedAt,
			},
		},
	}

	var output bytes.Buffer
	state := newTimelineState()
	if err := writeRunTimelineUpdate(&output, response, OutputOptions{}, state); err != nil {
		t.Fatalf("write timeline: %v", err)
	}

	for _, want := range []string{
		"agent [turn 2]",
		"tool_call",
		"sandbox_exec",
		"request:\n    command: wc -l /workspace/README.md",
		"response:\n    type: tool_call\n    tool: sandbox_exec",
		"message:\n    model requested tool call",
		"tool [tool 1]",
		"response:\n    12 /workspace/README.md",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("timeline missing %q:\n%s", want, output.String())
		}
	}
	if strings.Contains(output.String(), "TIME                SOURCE") {
		t.Fatalf("timeline still uses table header:\n%s", output.String())
	}
	if strings.Contains(output.String(), " run run_timeline ") {
		t.Fatalf("timeline should not print run rows with run IDs:\n%s", output.String())
	}
	for _, want := range []string{
		"Run: run_timeline\n\n",
		"    model requested tool call\n",
		"    12 /workspace/README.md\n\n",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("timeline missing blank-line spacing around %q:\n%s", want, output.String())
		}
	}
}

func TestWriteRunTimelineUpdateKeepsSubsecondEventOrder(t *testing.T) {
	base := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	response := RunResponse{
		ID:        "run_order",
		Status:    statusCompleted,
		UpdatedAt: base.Add(2 * time.Second),
		Result:    &RunResultResponse{Summary: "done"},
		AgentTurns: []AgentTurnResponse{
			{
				Index:     1,
				Status:    "tool_call",
				ToolName:  "sandbox_exec",
				Message:   "model requested tool call",
				StartedAt: base.Add(100 * time.Millisecond),
				EndedAt:   base.Add(200 * time.Millisecond),
			},
			{
				Index:     2,
				Status:    "tool_call",
				ToolName:  "sandbox_exec",
				Message:   "model requested tool call",
				StartedAt: base.Add(700 * time.Millisecond),
				EndedAt:   base.Add(800 * time.Millisecond),
			},
			{
				Index:     3,
				Status:    timelineStatusModelResponse,
				Message:   "waiting for model response",
				StartedAt: base.Add(950 * time.Millisecond),
			},
		},
		ToolCalls: []ToolCallResponse{
			{
				Name:      "sandbox_exec",
				Arguments: map[string]string{"command": "false"},
				Result:    "failed",
				IsError:   true,
				StartedAt: base.Add(250 * time.Millisecond),
				EndedAt:   base.Add(300 * time.Millisecond),
			},
			{
				Name:      "sandbox_exec",
				Arguments: map[string]string{"command": "true"},
				Result:    "ok",
				StartedAt: base.Add(850 * time.Millisecond),
				EndedAt:   base.Add(900 * time.Millisecond),
			},
		},
	}

	var output bytes.Buffer
	if err := writeRunTimelineUpdate(&output, response, OutputOptions{}, newTimelineState()); err != nil {
		t.Fatalf("write timeline: %v", err)
	}

	assertSubstringOrder(t, output.String(), "tool [tool 2]", "agent [turn 3]")
}

func TestWriteRunTimelineUpdateOrdersToolBeforeNextModelWaitWhenTimestampsMatch(t *testing.T) {
	at := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	response := RunResponse{
		ID:        "run_tie",
		Status:    statusCompleted,
		UpdatedAt: at.Add(time.Second),
		Result:    &RunResultResponse{Summary: "done"},
		AgentTurns: []AgentTurnResponse{
			{
				Index:     2,
				Status:    timelineStatusModelResponse,
				Message:   "waiting for model response",
				StartedAt: at,
			},
		},
		ToolCalls: []ToolCallResponse{
			{
				Name:      "sandbox_exec",
				Arguments: map[string]string{"command": "true"},
				Result:    "ok",
				StartedAt: at,
				EndedAt:   at,
			},
		},
	}

	var output bytes.Buffer
	if err := writeRunTimelineUpdate(&output, response, OutputOptions{}, newTimelineState()); err != nil {
		t.Fatalf("write timeline: %v", err)
	}

	assertSubstringOrder(t, output.String(), "tool [tool 1]", "agent [turn 2]")
}

func TestWriteRunTimelineUpdatePrintsFinalResultAfterLifecycle(t *testing.T) {
	startedAt := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	endedAt := startedAt.Add(time.Second)
	response := RunResponse{
		ID:        "run_final",
		Status:    statusCompleted,
		UpdatedAt: endedAt,
		Result:    &RunResultResponse{Summary: "done"},
		Steps: []StepResponse{
			{
				Name:      "agent",
				Status:    statusCompleted,
				Message:   "Agent generated result summary after 2 tool call(s)",
				StartedAt: startedAt,
				EndedAt:   &endedAt,
			},
		},
		AgentTurns: []AgentTurnResponse{
			{
				Index:           3,
				Status:          timelineActionFinal,
				ActionType:      timelineActionFinal,
				Message:         "model returned final answer",
				RawResponse:     `{"type":"final","summary":"done"}`,
				ResponsePreview: "done",
				StartedAt:       startedAt,
				EndedAt:         endedAt,
			},
		},
	}

	var output bytes.Buffer
	if err := writeRunTimelineUpdate(&output, response, OutputOptions{}, newTimelineState()); err != nil {
		t.Fatalf("write timeline: %v", err)
	}
	for _, want := range []string{
		"agent [turn 3]",
		"step agent step_status=completed action=lifecycle",
		"result status=completed action=final",
		"response:\n    done",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("timeline missing %q:\n%s", want, output.String())
		}
	}
	assertSubstringOrder(t, output.String(), "step agent step_status=completed action=lifecycle", "result status=completed action=final")
}

func TestWriteRunTimelineUpdateHidesNonDebugTransientStatus(t *testing.T) {
	response := RunResponse{ID: "run_running", Status: "running"}
	var output bytes.Buffer

	if err := writeRunTimelineUpdate(&output, response, OutputOptions{}, newTimelineState()); err != nil {
		t.Fatalf("write timeline: %v", err)
	}
	if strings.Contains(output.String(), "status=running") {
		t.Fatalf("timeline should hide running status outside debug:\n%s", output.String())
	}

	if err := writeRunTimelineUpdate(&output, response, OutputOptions{Debug: true}, newTimelineState()); err != nil {
		t.Fatalf("write debug timeline: %v", err)
	}
	if !strings.Contains(output.String(), "status=running") {
		t.Fatalf("debug timeline should include running run status:\n%s", output.String())
	}
	if strings.Contains(output.String(), " run run_running ") {
		t.Fatalf("debug timeline should not print run rows with run IDs:\n%s", output.String())
	}

	output.Reset()
	response = RunResponse{ID: "run_queued", Status: timelineStatusQueued}
	if err := writeRunTimelineUpdate(&output, response, OutputOptions{}, newTimelineState()); err != nil {
		t.Fatalf("write queued timeline: %v", err)
	}
	if strings.Contains(output.String(), "status=queued") {
		t.Fatalf("timeline should hide queued status outside debug:\n%s", output.String())
	}

	output.Reset()
	if err := writeRunTimelineUpdate(&output, response, OutputOptions{Debug: true}, newTimelineState()); err != nil {
		t.Fatalf("write debug queued timeline: %v", err)
	}
	if !strings.Contains(output.String(), "status=queued") {
		t.Fatalf("debug timeline should include queued status:\n%s", output.String())
	}
}

func TestWriteRunTimelineUpdateOnlyPrintsNewEntries(t *testing.T) {
	response := RunResponse{ID: "run_incremental", Status: "running"}
	var output bytes.Buffer
	state := newTimelineState()

	if err := writeRunTimelineUpdate(&output, response, OutputOptions{}, state); err != nil {
		t.Fatalf("write first timeline: %v", err)
	}
	first := output.String()
	if err := writeRunTimelineUpdate(&output, response, OutputOptions{}, state); err != nil {
		t.Fatalf("write second timeline: %v", err)
	}
	if output.String() != first {
		t.Fatalf("timeline wrote duplicate entries:\n%s", output.String())
	}
}

func TestWriteRunTimelineUpdateNormalizesCarriageReturns(t *testing.T) {
	startedAt := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	response := RunResponse{
		ID:        "run_carriage_return",
		Status:    statusCompleted,
		UpdatedAt: startedAt.Add(time.Second),
		Result:    &RunResultResponse{Summary: "done"},
		ToolCalls: []ToolCallResponse{
			{
				Name:      "sandbox_exec",
				Arguments: map[string]string{"command": "printf progress"},
				Result:    "progress 50%\rprogress 100%\r\nok",
				StartedAt: startedAt,
				EndedAt:   startedAt.Add(time.Second),
			},
		},
	}

	var output bytes.Buffer
	if err := writeRunTimelineUpdate(&output, response, OutputOptions{}, newTimelineState()); err != nil {
		t.Fatalf("write timeline: %v", err)
	}
	if strings.Contains(output.String(), "\r") {
		t.Fatalf("timeline should normalize carriage returns:\n%q", output.String())
	}
	for _, want := range []string{"progress 50%\n", "progress 100%\n", "ok\n"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("timeline missing normalized output %q:\n%s", want, output.String())
		}
	}
}

func TestTurnArgumentsReadsNativeToolCalls(t *testing.T) {
	arguments := turnArguments(AgentTurnResponse{
		ToolName:    "sandbox_exec",
		RawResponse: `{"type":"tool_call","tool_calls":[{"tool":"sandbox_exec","arguments":{"command":"pwd"}}]}`,
	})
	if arguments["command"] != "pwd" {
		t.Fatalf("command = %q, want pwd", arguments["command"])
	}
}

func assertSubstringOrder(t *testing.T, content string, first string, second string) {
	t.Helper()
	firstIndex := strings.Index(content, first)
	if firstIndex < 0 {
		t.Fatalf("output missing %q:\n%s", first, content)
	}
	secondIndex := strings.Index(content, second)
	if secondIndex < 0 {
		t.Fatalf("output missing %q:\n%s", second, content)
	}
	if firstIndex > secondIndex {
		t.Fatalf("%q should appear before %q:\n%s", first, second, content)
	}
}
