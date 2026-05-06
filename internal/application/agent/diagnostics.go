package agent

import (
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

const toolArgumentCommand = "command"

const agentTurnPreviewTruncatedMarker = "\n... [truncated]"

const (
	modelResponseFormatEmpty              = "empty"
	modelResponseFormatPlainText          = "plain_text"
	modelResponseFormatMarkdownFence      = "markdown_fence"
	modelResponseFormatJSONObject         = "json_object"
	modelResponseFormatJSONArray          = "json_array"
	modelResponseFormatJSONScalar         = "json_scalar"
	modelResponseFormatInvalidJSON        = "invalid_json"
	modelResponseFormatMultipleJSONValues = "multiple_json_values"
)

func copyToolCallRecords(records []ToolCallRecord) []ToolCallRecord {
	if len(records) == 0 {
		return nil
	}

	copied := make([]ToolCallRecord, 0, len(records))
	for _, record := range records {
		item := record
		item.Arguments = copyArguments(record.Arguments)
		copied = append(copied, item)
	}

	return copied
}

func copyTurnRecords(records []TurnRecord) []TurnRecord {
	if len(records) == 0 {
		return nil
	}

	copied := make([]TurnRecord, 0, len(records))
	for _, record := range records {
		item := record
		item.RequestMessages = copyTurnMessageRecords(record.RequestMessages)
		item.RawResponse = previewRawResponse(record.RawResponse)
		item.ResponsePreview = previewModelResponse(record.ResponsePreview)
		item.CorrectionMessage = previewCorrectionMessage(record.CorrectionMessage)
		copied = append(copied, item)
	}

	return copied
}

func copyArguments(arguments map[string]string) map[string]string {
	if len(arguments) == 0 {
		return nil
	}

	copied := make(map[string]string, len(arguments))
	for key, value := range arguments {
		copied[key] = value
	}

	return copied
}

func copyToolDefinitions(definitions []outbound.ToolDefinition) []outbound.ToolDefinition {
	if len(definitions) == 0 {
		return nil
	}

	copied := make([]outbound.ToolDefinition, 0, len(definitions))
	for _, definition := range definitions {
		item := definition
		if len(definition.Arguments) > 0 {
			item.Arguments = append([]outbound.ToolArgumentDefinition(nil), definition.Arguments...)
		}
		copied = append(copied, item)
	}

	return copied
}

func normalizeModelToolCalls(calls []outbound.ModelToolCall) []outbound.ModelToolCall {
	if len(calls) == 0 {
		return nil
	}

	normalized := make([]outbound.ModelToolCall, 0, len(calls))
	for index, call := range calls {
		name := strings.TrimSpace(call.Name)
		if name == "" {
			continue
		}
		id := strings.TrimSpace(call.ID)
		if id == "" {
			id = generatedToolCallID(name, index)
		}
		normalized = append(normalized, outbound.ModelToolCall{
			ID:        id,
			Name:      name,
			Arguments: copyArguments(call.Arguments),
		})
	}

	return normalized
}

func generatedToolCallID(name string, index int) string {
	normalized := strings.Map(func(char rune) rune {
		switch {
		case char >= 'a' && char <= 'z':
			return char
		case char >= 'A' && char <= 'Z':
			return char + ('a' - 'A')
		case char >= '0' && char <= '9':
			return char
		default:
			return '_'
		}
	}, name)
	normalized = strings.Trim(normalized, "_")
	if normalized == "" {
		normalized = "tool"
	}

	return normalized + "_" + strconv.Itoa(index+1)
}

func joinedToolCallNames(calls []outbound.ModelToolCall) string {
	if len(calls) == 0 {
		return ""
	}

	names := make([]string, 0, len(calls))
	for _, call := range calls {
		if name := strings.TrimSpace(call.Name); name != "" {
			names = append(names, name)
		}
	}

	return strings.Join(names, ",")
}

func nativeToolCallsRawResponse(calls []outbound.ModelToolCall) string {
	type nativeToolCallRecord struct {
		ID        string            `json:"id,omitempty"`
		Tool      string            `json:"tool"`
		Arguments map[string]string `json:"arguments,omitempty"`
	}
	payload := struct {
		Type      string                 `json:"type"`
		ToolCalls []nativeToolCallRecord `json:"tool_calls"`
	}{
		Type:      string(outbound.ModelPartKindToolCall),
		ToolCalls: make([]nativeToolCallRecord, 0, len(calls)),
	}
	for _, call := range calls {
		name := strings.TrimSpace(call.Name)
		if name == "" {
			continue
		}
		payload.ToolCalls = append(payload.ToolCalls, nativeToolCallRecord{
			ID:        strings.TrimSpace(call.ID),
			Tool:      name,
			Arguments: copyArguments(call.Arguments),
		})
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return `{"type":"tool_call","tool_calls":[]}`
	}

	return string(encoded)
}

func responseRequestMessages(response outbound.ModelResponse, fallback []TurnMessageRecord) []TurnMessageRecord {
	messages := copyProviderRequestMessages(response.RequestMessages)
	if len(messages) == 0 {
		return fallback
	}

	return messages
}

func copyProviderRequestMessages(messages []outbound.ModelRequestMessage) []TurnMessageRecord {
	if len(messages) == 0 {
		return nil
	}

	copied := make([]TurnMessageRecord, 0, len(messages))
	for _, message := range messages {
		copied = append(copied, TurnMessageRecord{
			Role:       message.Role,
			Kind:       message.Kind,
			Content:    message.Content,
			ToolCallID: message.ToolCallID,
			ToolName:   message.ToolName,
		})
	}

	return copyTurnMessageRecords(copied)
}

func copyModelRequestMessages(instructions string, turns []outbound.ModelTurn) []TurnMessageRecord {
	var copied []TurnMessageRecord
	if strings.TrimSpace(instructions) != "" {
		copied = append(copied, TurnMessageRecord{
			Role:    "system",
			Kind:    "system_prompt",
			Content: instructions,
		})
	}
	for _, turn := range turns {
		role := string(turn.Role)
		for _, part := range turn.Parts {
			copied = append(copied, TurnMessageRecord{
				Role:       role,
				Kind:       string(part.Kind),
				Content:    part.Text,
				ToolCallID: part.ToolCallID,
				ToolName:   part.ToolName,
			})
		}
	}

	return copyTurnMessageRecords(copied)
}

func copyTurnMessageRecords(messages []TurnMessageRecord) []TurnMessageRecord {
	if len(messages) == 0 {
		return nil
	}

	copied := make([]TurnMessageRecord, 0, len(messages))
	for _, message := range messages {
		role := strings.TrimSpace(message.Role)
		if role == "" {
			continue
		}

		copied = append(copied, TurnMessageRecord{
			Role:       role,
			Kind:       strings.TrimSpace(message.Kind),
			Content:    previewRequestMessageContent(message.Content),
			ToolCallID: strings.TrimSpace(message.ToolCallID),
			ToolName:   strings.TrimSpace(message.ToolName),
		})
	}

	return copied
}

func availableToolMap(tools []outbound.ToolDefinition) map[string]outbound.ToolDefinition {
	if len(tools) == 0 {
		return map[string]outbound.ToolDefinition{}
	}

	indexed := make(map[string]outbound.ToolDefinition, len(tools))
	for _, tool := range tools {
		indexed[tool.Name] = tool
	}

	return indexed
}

func availableToolNames(tools []outbound.ToolDefinition) []string {
	if len(tools) == 0 {
		return nil
	}

	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)

	return names
}

func placeholderArgumentValues(arguments map[string]string) []string {
	if len(arguments) == 0 {
		return nil
	}

	var placeholders []string
	for name, value := range arguments {
		for _, token := range placeholderTokens(value) {
			placeholders = append(placeholders, name+"="+token)
		}
	}
	sort.Strings(placeholders)

	return placeholders
}

func placeholderTokens(value string) []string {
	var tokens []string
	remaining := value
	for {
		start := strings.Index(remaining, "<")
		if start < 0 {
			break
		}
		remaining = remaining[start+1:]
		end := strings.Index(remaining, ">")
		if end < 0 {
			break
		}
		name := remaining[:end]
		token := "<" + name + ">"
		if isPlaceholderName(name) {
			tokens = append(tokens, token)
		}
		remaining = remaining[end+1:]
	}

	return tokens
}

func isPlaceholderName(name string) bool {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if normalized == "" || len(normalized) > 64 {
		return false
	}
	for _, char := range normalized {
		if (char >= 'a' && char <= 'z') ||
			(char >= '0' && char <= '9') ||
			char == '_' ||
			char == '-' ||
			char == ' ' {
			continue
		}

		return false
	}

	compact := strings.NewReplacer("_", "", "-", "", " ", "").Replace(normalized)
	switch compact {
	case toolArgumentCommand, "filename", "filepath", "key", "path", "toolname", "value":
		return true
	default:
		return strings.Contains(compact, "file") || strings.Contains(compact, "path")
	}
}

func classifyModelResponseFormat(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return modelResponseFormatEmpty
	}
	if strings.HasPrefix(trimmed, "```") {
		return modelResponseFormatMarkdownFence
	}

	decoded, extraErr, decodeErr := decodeSingleJSONValue(trimmed)
	if decodeErr != nil {
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			return modelResponseFormatInvalidJSON
		}

		return modelResponseFormatPlainText
	}
	if !errors.Is(extraErr, io.EOF) {
		return modelResponseFormatMultipleJSONValues
	}

	switch decoded.(type) {
	case map[string]any:
		return modelResponseFormatJSONObject
	case []any:
		return modelResponseFormatJSONArray
	default:
		return modelResponseFormatJSONScalar
	}
}

func previewModelResponse(content string) string {
	return truncateAgentTurnText(content, run.MaxAgentTurnPreviewLength)
}

func previewRawResponse(content string) string {
	return truncateAgentTurnText(content, run.MaxAgentTurnRawResponseLength)
}

func previewRequestMessageContent(content string) string {
	return truncateAgentTurnText(content, run.MaxAgentTurnMessageContentLength)
}

func boundedAssistantAttempt(content string) string {
	return truncateAgentTurnText(content, run.MaxAgentTurnPreviewLength)
}

func previewCorrectionMessage(content string) string {
	return truncateAgentTurnText(strings.TrimSpace(content), run.MaxAgentTurnCorrectionLength)
}

func truncateAgentTurnText(content string, maxLength int) string {
	content = strings.ToValidUTF8(content, "\uFFFD")
	if len(content) <= maxLength {
		return content
	}

	maxContentLength := maxLength - len(agentTurnPreviewTruncatedMarker)
	if maxContentLength < 0 {
		maxContentLength = 0
	}

	for maxContentLength > 0 && !utf8.ValidString(content[:maxContentLength]) {
		maxContentLength--
	}

	return content[:maxContentLength] + agentTurnPreviewTruncatedMarker
}
