package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	timelineTimeLayout          = "2006-01-02 15:04:05"
	timelineMissingValue        = "-"
	timelineSourceAgent         = "agent"
	timelineSourceArtifact      = "artifact"
	timelineSourceFailure       = "failure"
	timelineSourceResult        = "result"
	timelineSourceStatus        = "status"
	timelineSourceStep          = "step"
	timelineSourceTool          = "tool"
	timelineStatusAvailable     = "available"
	timelineStatusQueued        = "queued"
	timelineStatusModelResponse = "model_response"
	timelineStatusModelWaiting  = "model_waiting"
	timelineActionFailure       = "failure"
	timelineActionFinal         = "final"
	timelineActionLifecycle     = "lifecycle"
	timelineActionModel         = "model"
	timelineActionOutput        = "output"
	timelineDetailNoResult      = "no result content"
	timelineKeyArtifact         = "artifact:"
	timelineKeyFailure          = "failure:"
	timelineKeyResult           = "result:"
	timelineKeyRun              = "run:"
	timelineKeyStep             = "step:"
	timelineKeyTool             = "tool:"
	timelineKeyTurn             = "turn:"
	timelineArgumentCommand     = "command"
	timelineFieldAction         = "action"
	timelineFieldStatus         = "status"
	timelineFieldAgentStatus    = "turn_status"
	timelineFieldStepStatus     = "step_status"
	timelineFieldToolStatus     = "tool_status"
	timelineFieldMessage        = "message"
	timelineFieldRequest        = "request"
	timelineFieldResponse       = "response"
	timelineResponsePreview     = "response_preview"
	timelineResponseFormat      = "response_format"
	timelineProtocolError       = "protocol_error_code"
	timelineTruncatedMarker     = "..."
	timelineDetailSeparator     = "; "
	timelineRefToolFormat       = "[tool %d]"
	timelineRefTurnFormat       = "[turn %d]"
	timelineMaxLineLength       = 220
	timelineMaxBlockLength      = 1200
	timelineMaxDebugBlockLength = 4000
	timelineMaxArgumentCount    = 3

	ansiReset   = "\x1b[0m"
	ansiBold    = "\x1b[1m"
	ansiDim     = "\x1b[2m"
	ansiRed     = "\x1b[31m"
	ansiGreen   = "\x1b[32m"
	ansiYellow  = "\x1b[33m"
	ansiBlue    = "\x1b[34m"
	ansiMagenta = "\x1b[35m"
	ansiCyan    = "\x1b[36m"
)

type timelineState struct {
	wroteHeader bool
	seen        map[string]struct{}
}

type timelineEntry struct {
	key        string
	occurredAt time.Time
	sequence   int
	source     string
	ref        string
	status     string
	action     string
	message    string
	request    map[string]string
	response   string
	debug      map[string]string
}

type timelineActionPayload struct {
	Type      string                `json:"type"`
	Tool      string                `json:"tool"`
	Summary   string                `json:"summary"`
	Arguments map[string]string     `json:"arguments"`
	ToolCalls []timelineToolPayload `json:"tool_calls"`
}

type timelineToolPayload struct {
	Tool      string            `json:"tool"`
	Arguments map[string]string `json:"arguments"`
}

func watchRun(
	ctx context.Context,
	client *Client,
	id string,
	initial *RunResponse,
	pollInterval time.Duration,
	stdout io.Writer,
	options OutputOptions,
) (RunResponse, error) {
	if pollInterval <= 0 {
		pollInterval = defaultPollInterval
	}

	state := newTimelineState()
	response, err := firstWatchResponse(ctx, client, id, initial)
	if err != nil {
		return RunResponse{}, err
	}
	if err := writeRunTimelineUpdate(stdout, response, options, state); err != nil {
		return RunResponse{}, err
	}
	for !response.Terminal() {
		if err := waitForPoll(ctx, pollInterval); err != nil {
			return RunResponse{}, err
		}
		response, err = client.GetRun(ctx, id)
		if err != nil {
			return RunResponse{}, err
		}
		if err := writeRunTimelineUpdate(stdout, response, options, state); err != nil {
			return RunResponse{}, err
		}
	}

	return response, nil
}

func firstWatchResponse(
	ctx context.Context,
	client *Client,
	id string,
	initial *RunResponse,
) (RunResponse, error) {
	if initial != nil {
		return *initial, nil
	}

	return client.GetRun(ctx, id)
}

func newTimelineState() *timelineState {
	return &timelineState{seen: map[string]struct{}{}}
}

func writeRunTimelineUpdate(
	writer io.Writer,
	response RunResponse,
	options OutputOptions,
	state *timelineState,
) error {
	if state == nil {
		state = newTimelineState()
	}

	var builder strings.Builder
	if !state.wroteHeader {
		fmt.Fprintf(&builder, "Run: %s\n\n", response.ID)
		state.wroteHeader = true
	}
	color := timelineColorEnabled(writer)
	for _, entry := range timelineEntries(response, options) {
		if !state.markSeen(entry.key) {
			continue
		}
		builder.WriteString(formatTimelineRow(entry, color))
	}
	_, err := io.WriteString(writer, builder.String())

	return err
}

func (s *timelineState) markSeen(key string) bool {
	if _, ok := s.seen[key]; ok {
		return false
	}
	s.seen[key] = struct{}{}

	return true
}

func timelineEntries(response RunResponse, options OutputOptions) []timelineEntry {
	entries := make([]timelineEntry, 0, timelineEntryCapacity(response))
	failure := failureTimelineEntry(response)
	if shouldIncludeRunStatus(response, options) && failure.key == "" {
		addTimelineEntry(&entries, runStatusEntry(response))
	}
	for _, step := range response.Steps {
		addTimelineEntry(&entries, stepTimelineEntry(step))
	}
	for _, turn := range response.AgentTurns {
		addTimelineEntry(&entries, agentTurnTimelineEntry(turn, options))
	}
	for index, call := range response.ToolCalls {
		addTimelineEntry(&entries, toolCallTimelineEntry(index+1, call))
	}
	if response.Result != nil && response.Result.Summary != "" {
		addTimelineEntry(&entries, resultTimelineEntry(response))
	}
	if failure.key != "" {
		addTimelineEntry(&entries, failure)
	}
	for _, artifact := range response.Artifacts {
		addTimelineEntry(&entries, artifactTimelineEntry(response, artifact))
	}
	sort.SliceStable(entries, func(i int, j int) bool {
		if entries[i].occurredAt.IsZero() != entries[j].occurredAt.IsZero() {
			return !entries[i].occurredAt.IsZero()
		}
		if !entries[i].occurredAt.Equal(entries[j].occurredAt) {
			return entries[i].occurredAt.Before(entries[j].occurredAt)
		}
		leftPriority := timelineSortPriority(entries[i])
		rightPriority := timelineSortPriority(entries[j])
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}

		return entries[i].sequence < entries[j].sequence
	})

	return entries
}

func shouldIncludeRunStatus(response RunResponse, options OutputOptions) bool {
	if options.Debug {
		return true
	}
	switch response.Status {
	case statusCancelled, statusFailed:
		return true
	case statusCompleted:
		return response.Result == nil || response.Result.Summary == ""
	default:
		return false
	}
}

func timelineSortPriority(entry timelineEntry) int {
	switch entry.source {
	case timelineSourceStatus:
		return 0
	case timelineSourceStep:
		return 10
	case timelineSourceAgent:
		if entry.status == timelineStatusModelResponse || entry.status == timelineActionFinal {
			return 35
		}
		return 20
	case timelineSourceTool:
		return 30
	case timelineSourceFailure:
		return 40
	case timelineSourceResult:
		return 45
	case timelineSourceArtifact:
		return 50
	default:
		return 60
	}
}

func timelineEntryCapacity(response RunResponse) int {
	return 3 + len(response.Steps) + len(response.AgentTurns) + len(response.ToolCalls) + len(response.Artifacts)
}

func addTimelineEntry(entries *[]timelineEntry, entry timelineEntry) {
	entry.sequence = len(*entries)
	*entries = append(*entries, entry)
}

func runStatusEntry(response RunResponse) timelineEntry {
	return timelineEntry{
		key:        timelineKeyRun + response.Status,
		occurredAt: runStatusTime(response),
		source:     timelineSourceStatus,
		status:     nonEmpty(response.Status),
		message:    runStatusDetail(response),
	}
}

func stepTimelineEntry(step StepResponse) timelineEntry {
	return timelineEntry{
		key:        timelineKeyStep + strings.Join(compactParts(step.Name, step.Status, step.Message), "|"),
		occurredAt: stepTime(step),
		source:     timelineSourceStep,
		ref:        nonEmpty(step.Name),
		status:     nonEmpty(step.Status),
		action:     timelineActionLifecycle,
		message:    step.Message,
	}
}

func agentTurnTimelineEntry(turn AgentTurnResponse, options OutputOptions) timelineEntry {
	return timelineEntry{
		key:        timelineKeyTurn + strconv.Itoa(turn.Index) + "|" + strings.Join(compactParts(turn.Status, turn.ActionType, turn.ToolName, turn.Message), "|"),
		occurredAt: turnTime(turn),
		source:     timelineSourceAgent,
		ref:        fmt.Sprintf(timelineRefTurnFormat, turn.Index),
		status:     nonEmpty(turn.Status),
		action:     agentTurnAction(turn),
		message:    turn.Message,
		request:    turnArguments(turn),
		response:   agentTurnResponse(turn),
		debug:      agentTurnDebugFields(turn, options),
	}
}

func toolCallTimelineEntry(index int, call ToolCallResponse) timelineEntry {
	return timelineEntry{
		key:        timelineKeyTool + strconv.Itoa(index) + "|" + strings.Join(compactParts(call.Name, toolCallStatus(call), call.StartedAt.String()), "|"),
		occurredAt: toolCallTime(call),
		source:     timelineSourceTool,
		ref:        fmt.Sprintf(timelineRefToolFormat, index),
		status:     toolCallStatus(call),
		action:     nonEmpty(call.Name),
		request:    copyStringMap(call.Arguments),
		response:   toolCallResponse(call),
	}
}

func resultTimelineEntry(response RunResponse) timelineEntry {
	return timelineEntry{
		key:        timelineKeyResult + response.Result.Summary,
		occurredAt: response.UpdatedAt,
		source:     timelineSourceResult,
		status:     nonEmpty(response.Status),
		action:     timelineActionFinal,
		response:   response.Result.Summary,
	}
}

func failureTimelineEntry(response RunResponse) timelineEntry {
	detail := failureDetail(response)
	if detail == "" {
		return timelineEntry{}
	}

	return timelineEntry{
		key:        timelineKeyFailure + detail,
		occurredAt: response.UpdatedAt,
		source:     timelineSourceFailure,
		status:     nonEmpty(response.Status),
		action:     timelineActionFailure,
		response:   detail,
	}
}

func artifactTimelineEntry(response RunResponse, artifact ArtifactResponse) timelineEntry {
	return timelineEntry{
		key:        timelineKeyArtifact + artifact.Path,
		occurredAt: response.UpdatedAt,
		source:     timelineSourceArtifact,
		ref:        nonEmpty(artifact.Path),
		status:     timelineStatusAvailable,
		action:     timelineActionOutput,
		message:    artifactDetail(artifact),
	}
}

func formatTimelineRow(entry timelineEntry, color bool) string {
	header := compactParts(
		colorize(formatTimelineTime(entry.occurredAt), ansiDim, color),
		colorize(entry.source, sourceColor(entry.source), color),
		colorize(entry.ref, ansiDim, color),
		formatLogField(timelineStatusField(entry.source), timelineDisplayStatus(entry), statusColor(entry.status), color),
		formatLogField(timelineFieldAction, timelineDisplayAction(entry), actionColor(entry.action), color),
	)

	var builder strings.Builder
	builder.WriteString(strings.Join(header, " "))
	builder.WriteString("\n")
	writeTimelineField(&builder, timelineFieldMessage, entry.message, false, color)
	writeTimelineMap(&builder, timelineFieldRequest, entry.request, false, color)
	writeTimelineField(&builder, timelineFieldResponse, entry.response, false, color)
	writeTimelineMap(&builder, "debug", entry.debug, true, color)
	builder.WriteString("\n")

	return builder.String()
}

func formatTimelineTime(value time.Time) string {
	if value.IsZero() {
		return timelineMissingValue
	}

	return value.Format(timelineTimeLayout)
}

func timelineDisplayStatus(entry timelineEntry) string {
	if entry.source == timelineSourceAgent && entry.status == timelineStatusModelResponse {
		return timelineStatusModelWaiting
	}

	return entry.status
}

func timelineDisplayAction(entry timelineEntry) string {
	if entry.source == timelineSourceAgent && entry.status == timelineStatusModelResponse {
		return timelineActionModel
	}

	return entry.action
}

func timelineStatusField(source string) string {
	switch source {
	case timelineSourceStep:
		return timelineFieldStepStatus
	case timelineSourceAgent:
		return timelineFieldAgentStatus
	case timelineSourceTool:
		return timelineFieldToolStatus
	default:
		return timelineFieldStatus
	}
}

func runStatusTime(response RunResponse) time.Time {
	if response.Status == timelineStatusQueued && !response.CreatedAt.IsZero() {
		return response.CreatedAt
	}
	if !response.UpdatedAt.IsZero() {
		return response.UpdatedAt
	}

	return response.CreatedAt
}

func stepTime(step StepResponse) time.Time {
	if step.EndedAt != nil && !step.EndedAt.IsZero() {
		return *step.EndedAt
	}

	return step.StartedAt
}

func turnTime(turn AgentTurnResponse) time.Time {
	if !turn.EndedAt.IsZero() {
		return turn.EndedAt
	}

	return turn.StartedAt
}

func toolCallTime(call ToolCallResponse) time.Time {
	if !call.EndedAt.IsZero() {
		return call.EndedAt
	}

	return call.StartedAt
}

func runStatusDetail(response RunResponse) string {
	if detail := failureDetail(response); detail != "" {
		return detail
	}

	return ""
}

func failureDetail(response RunResponse) string {
	return strings.Join(compactParts(response.FailureCode, response.FailureMessage, response.FailureReason), timelineDetailSeparator)
}

func artifactDetail(artifact ArtifactResponse) string {
	parts := []string{fmt.Sprintf("%d bytes", artifact.SizeBytes)}
	if artifact.MediaType != "" {
		parts = append(parts, artifact.MediaType)
	}

	return strings.Join(parts, timelineDetailSeparator)
}

func agentTurnAction(turn AgentTurnResponse) string {
	if turn.ToolName != "" {
		return turn.ToolName
	}
	if turn.ActionType != "" {
		return turn.ActionType
	}
	if turn.ResponseFormat != "" {
		return turn.ResponseFormat
	}

	return timelineSourceAgent
}

func agentTurnResponse(turn AgentTurnResponse) string {
	content := firstNonEmpty(turn.RawResponse, turn.ResponsePreview)
	if content == "" {
		return ""
	}

	if formatted := formattedModelResponse(content); formatted != "" {
		return formatted
	}

	return content
}

func formattedModelResponse(content string) string {
	var payload timelineActionPayload
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return ""
	}

	lines := compactParts(formatPrefixedValue("type", payload.Type))
	if payload.Tool != "" {
		lines = append(lines, formatPrefixedValue("tool", payload.Tool))
	}
	if payload.Summary != "" {
		lines = append(lines, formatPrefixedValue("summary", payload.Summary))
	}
	for index, call := range payload.ToolCalls {
		if call.Tool == "" {
			continue
		}
		lines = append(lines, formatPrefixedValue(fmt.Sprintf("tool_call[%d]", index+1), call.Tool))
	}

	return strings.Join(lines, "\n")
}

func agentTurnDebugFields(turn AgentTurnResponse, options OutputOptions) map[string]string {
	if !options.Debug {
		return nil
	}

	fields := map[string]string{}
	addMapValue(fields, timelineResponseFormat, turn.ResponseFormat)
	addMapValue(fields, timelineProtocolError, turn.ProtocolErrorCode)
	addMapValue(fields, timelineResponsePreview, turn.ResponsePreview)
	addMapValue(fields, "raw_response", turn.RawResponse)
	addMapValue(fields, "correction_message", turn.CorrectionMessage)

	return fields
}

func toolCallResponse(call ToolCallResponse) string {
	if call.Result != "" {
		return call.Result
	}

	return timelineDetailNoResult
}

func turnArguments(turn AgentTurnResponse) map[string]string {
	content := strings.TrimSpace(turn.RawResponse)
	if content == "" {
		content = strings.TrimSpace(turn.ResponsePreview)
	}
	if content == "" {
		return nil
	}

	var payload timelineActionPayload
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return nil
	}
	if len(payload.Arguments) > 0 {
		return payload.Arguments
	}
	for _, call := range payload.ToolCalls {
		if turn.ToolName == "" || turn.ToolName == call.Tool || strings.Contains(turn.ToolName, call.Tool) {
			return call.Arguments
		}
	}

	return nil
}

func sortedArgumentKeys(arguments map[string]string) []string {
	if len(arguments) == 0 {
		return nil
	}

	seen := map[string]struct{}{}
	keys := make([]string, 0, len(arguments))
	for _, key := range preferredArgumentKeys() {
		if _, ok := arguments[key]; ok {
			keys = append(keys, key)
			seen[key] = struct{}{}
		}
	}
	remaining := make([]string, 0, len(arguments))
	for key := range arguments {
		if _, ok := seen[key]; !ok {
			remaining = append(remaining, key)
		}
	}
	sort.Strings(remaining)

	return append(keys, remaining...)
}

func preferredArgumentKeys() []string {
	return []string{
		timelineArgumentCommand,
		"operation",
		"path",
		"area",
		"timeout_seconds",
		"max_output_bytes",
	}
}

func formatLogField(name string, value string, colorCode string, color bool) string {
	if strings.TrimSpace(value) == "" || value == timelineMissingValue {
		return ""
	}

	return name + "=" + colorize(value, colorCode, color)
}

func writeTimelineField(builder *strings.Builder, label string, value string, debug bool, color bool) {
	value = boundedTimelineBlock(value, debug)
	if value == "" {
		return
	}

	fmt.Fprintf(builder, "  %s:\n", colorize(label, ansiBold, color))
	for _, line := range strings.Split(value, "\n") {
		fmt.Fprintf(builder, "    %s\n", line)
	}
}

func writeTimelineMap(builder *strings.Builder, label string, values map[string]string, debug bool, color bool) {
	keys := sortedArgumentKeys(values)
	if len(keys) == 0 {
		return
	}
	if !debug && len(keys) > timelineMaxArgumentCount {
		keys = keys[:timelineMaxArgumentCount]
	}
	fmt.Fprintf(builder, "  %s:\n", colorize(label, ansiBold, color))
	for _, key := range keys {
		fmt.Fprintf(builder, "    %s: %s\n", colorize(key, ansiDim, color), boundedTimelineLine(values[key], debug))
	}
	if !debug && len(values) > len(keys) {
		fmt.Fprintf(builder, "    %s\n", timelineTruncatedMarker)
	}
}

func boundedTimelineLine(value string, debug bool) string {
	value = strings.Join(strings.Fields(normalizeTimelineText(value)), " ")
	return truncateTimelineText(value, timelineLineLimit(debug))
}

func boundedTimelineBlock(value string, debug bool) string {
	value = strings.TrimSpace(normalizeTimelineText(value))
	if value == "" {
		return ""
	}

	return truncateTimelineText(value, timelineBlockLimit(debug))
}

func normalizeTimelineText(value string) string {
	value = strings.ToValidUTF8(value, "\uFFFD")
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")

	return value
}

func truncateTimelineText(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	if limit <= len(timelineTruncatedMarker) {
		return timelineTruncatedMarker
	}

	return strings.TrimSpace(value[:limit-len(timelineTruncatedMarker)]) + timelineTruncatedMarker
}

func timelineLineLimit(debug bool) int {
	limit := timelineMaxLineLength
	if debug {
		limit = timelineMaxDebugBlockLength
	}

	return limit
}

func timelineBlockLimit(debug bool) int {
	limit := timelineMaxBlockLength
	if debug {
		limit = timelineMaxDebugBlockLength
	}

	return limit
}

func copyStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}

	copied := make(map[string]string, len(values))
	for key, value := range values {
		copied[key] = value
	}

	return copied
}

func addMapValue(values map[string]string, key string, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}

	values[key] = value
}

func formatPrefixedValue(name string, value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}

	return name + ": " + value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}

	return ""
}

func timelineColorEnabled(writer io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}

	return info.Mode()&os.ModeCharDevice != 0
}

func sourceColor(source string) string {
	switch source {
	case timelineSourceAgent:
		return ansiMagenta
	case timelineSourceTool:
		return ansiCyan
	case timelineSourceStep:
		return ansiBlue
	case timelineSourceStatus:
		return ansiDim
	case timelineSourceResult:
		return ansiGreen
	case timelineSourceFailure:
		return ansiRed
	default:
		return ansiBold
	}
}

func statusColor(status string) string {
	switch status {
	case statusCompleted, "ok", timelineActionFinal:
		return ansiGreen
	case statusFailed, "error", "tool_error", "model_error", "protocol_error", "invalid_tool_call":
		return ansiRed
	case "running", "preparing", timelineStatusQueued, timelineStatusModelResponse, timelineStatusModelWaiting, "tool_call":
		return ansiYellow
	default:
		return ansiBold
	}
}

func actionColor(action string) string {
	switch action {
	case "sandbox_exec":
		return ansiCyan
	case "workspace":
		return ansiBlue
	case timelineActionModel:
		return ansiMagenta
	case timelineActionFinal:
		return ansiGreen
	case timelineActionFailure:
		return ansiRed
	default:
		return ansiBold
	}
}

func colorize(value string, code string, enabled bool) string {
	if !enabled || value == "" || code == "" {
		return value
	}

	return code + value + ansiReset
}

func nonEmpty(value string) string {
	if strings.TrimSpace(value) == "" {
		return timelineMissingValue
	}

	return value
}
