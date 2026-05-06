package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

const (
	sectionAgentPrompt  = "Agent prompt:"
	sectionAgentTurns   = "Agent turns:"
	sectionArtifacts    = "Artifacts:"
	sectionFailure      = "Failure:"
	sectionResult       = "Result:"
	sectionSteps        = "Steps:"
	sectionToolCalls    = "Tool calls:"
	formatTwoValuesLine = "%s %s\n"
	statusLabel         = "Status:"
)

// WriteRunOutput writes one run in JSON or human-readable form.
func WriteRunOutput(writer io.Writer, response RunResponse, options OutputOptions) error {
	if options.JSON {
		return writeJSON(writer, response)
	}
	_, err := writer.Write([]byte(FormatRun(response, options)))

	return err
}

// WriteRunsOutput writes a run list in JSON or human-readable form.
func WriteRunsOutput(writer io.Writer, responses []RunResponse, options OutputOptions) error {
	if options.JSON {
		return writeJSON(writer, responses)
	}
	_, err := writer.Write([]byte(FormatRuns(responses, options)))

	return err
}

// WriteArtifactsOutput writes artifact metadata in JSON or human-readable form.
func WriteArtifactsOutput(writer io.Writer, response ArtifactsResponse, options OutputOptions) error {
	if options.JSON {
		return writeJSON(writer, response)
	}
	_, err := writer.Write([]byte(FormatArtifacts(response)))

	return err
}

// FormatRun returns a human-readable run summary.
func FormatRun(response RunResponse, options OutputOptions) string {
	var buffer bytes.Buffer
	fmt.Fprintf(&buffer, "Run: %s\n", response.ID)
	fmt.Fprintf(&buffer, formatTwoValuesLine, statusLabel, response.Status)
	writeFailure(&buffer, response)
	writeResult(&buffer, response)
	writeArtifacts(&buffer, response.Artifacts)
	writeSteps(&buffer, response.Steps)
	writeAgentTurns(&buffer, response.AgentTurns, options.Debug)
	writeToolCalls(&buffer, response.ToolCalls, options.Debug)
	if options.Debug && hasAgentPromptMetadata(response) {
		writeSectionHeader(&buffer, sectionAgentPrompt)
		if response.AgentPromptVersion != "" {
			fmt.Fprintf(&buffer, "version: %s\n", response.AgentPromptVersion)
		}
		if response.AgentPromptSHA256 != "" {
			fmt.Fprintf(&buffer, "sha256: %s\n", response.AgentPromptSHA256)
		}
		if response.AgentSystemPrompt != "" {
			fmt.Fprintf(&buffer, "system_prompt:\n%s\n", response.AgentSystemPrompt)
		}
	}

	return buffer.String()
}

func hasAgentPromptMetadata(response RunResponse) bool {
	return response.AgentPromptVersion != "" ||
		response.AgentPromptSHA256 != "" ||
		response.AgentSystemPrompt != ""
}

// FormatArtifacts returns a human-readable artifact list.
func FormatArtifacts(response ArtifactsResponse) string {
	if len(response.Artifacts) == 0 {
		return "Artifacts: none\n"
	}

	var buffer bytes.Buffer
	buffer.WriteString(sectionArtifacts)
	buffer.WriteString("\n")
	for _, artifact := range response.Artifacts {
		if artifact.MediaType != "" {
			fmt.Fprintf(&buffer, "- %s (%d bytes, %s)\n", artifact.Path, artifact.SizeBytes, artifact.MediaType)
			continue
		}
		fmt.Fprintf(&buffer, "- %s (%d bytes)\n", artifact.Path, artifact.SizeBytes)
	}

	return buffer.String()
}

// FormatRuns returns a human-readable run list.
func FormatRuns(responses []RunResponse, options OutputOptions) string {
	if len(responses) == 0 {
		return "Runs: none\n"
	}

	var buffer bytes.Buffer
	for index, response := range responses {
		if index > 0 {
			buffer.WriteString("\n")
		}
		if options.Debug {
			buffer.WriteString(FormatRun(response, options))
			continue
		}
		fmt.Fprintf(&buffer, "Run: %s\n", response.ID)
		fmt.Fprintf(&buffer, formatTwoValuesLine, statusLabel, response.Status)
		writeFailure(&buffer, response)
		if response.Result != nil && response.Result.Summary != "" {
			fmt.Fprintf(&buffer, "Summary: %s\n", response.Result.Summary)
		}
	}

	return buffer.String()
}

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")

	return encoder.Encode(value)
}

func writeFailure(buffer *bytes.Buffer, response RunResponse) {
	if response.FailureCode == "" && response.FailureMessage == "" && response.FailureReason == "" {
		return
	}

	buffer.WriteString("\n")
	if response.FailureCode != "" || response.FailureMessage != "" {
		fmt.Fprintf(buffer, "%s %s", sectionFailure, response.FailureCode)
		if response.FailureMessage != "" {
			fmt.Fprintf(buffer, " - %s", response.FailureMessage)
		}
		buffer.WriteString("\n")

		return
	}
	fmt.Fprintf(buffer, formatTwoValuesLine, sectionFailure, response.FailureReason)
}

func writeResult(buffer *bytes.Buffer, response RunResponse) {
	if response.Result == nil || response.Result.Summary == "" {
		return
	}

	writeSectionHeader(buffer, sectionResult)
	fmt.Fprintf(buffer, "%s\n", response.Result.Summary)
}

func writeSteps(buffer *bytes.Buffer, steps []StepResponse) {
	if len(steps) == 0 {
		return
	}

	writeSectionHeader(buffer, sectionSteps)
	for _, step := range steps {
		line := step.Name
		if step.Status != "" {
			line += " - " + step.Status
		}
		if step.Message != "" {
			line += " - " + step.Message
		}
		fmt.Fprintf(buffer, "- %s\n", line)
	}
}

func writeArtifacts(buffer *bytes.Buffer, artifacts []ArtifactResponse) {
	if len(artifacts) == 0 {
		return
	}

	writeSectionHeader(buffer, sectionArtifacts)
	for _, artifact := range artifacts {
		if artifact.MediaType != "" {
			fmt.Fprintf(buffer, "- %s (%d bytes, %s)\n", artifact.Path, artifact.SizeBytes, artifact.MediaType)
			continue
		}
		fmt.Fprintf(buffer, "- %s (%d bytes)\n", artifact.Path, artifact.SizeBytes)
	}
}

func writeAgentTurns(buffer *bytes.Buffer, turns []AgentTurnResponse, debug bool) {
	if len(turns) == 0 {
		return
	}

	writeSectionHeader(buffer, sectionAgentTurns)
	for _, turn := range turns {
		line := agentTurnLine(turn)
		if debug {
			line = agentTurnDebugLine(turn)
		}
		fmt.Fprintf(buffer, "%d. %s\n", turn.Index, line)
	}
}

func writeToolCalls(buffer *bytes.Buffer, calls []ToolCallResponse, debug bool) {
	if len(calls) == 0 {
		return
	}

	writeSectionHeader(buffer, sectionToolCalls)
	for _, call := range calls {
		if debug {
			fmt.Fprintf(buffer, "- %s: %s\n", call.Name, toolCallStatus(call))
			writeArguments(buffer, call.Arguments)
			if call.Result != "" {
				fmt.Fprintf(buffer, "  result: %s\n", call.Result)
			}
			continue
		}
		fmt.Fprintf(buffer, "- %s: %s\n", call.Name, toolCallStatus(call))
	}
}

func agentTurnLine(turn AgentTurnResponse) string {
	parts := compactParts(turn.Status, turn.ToolName, turn.Message)
	if len(parts) == 0 {
		return "turn"
	}

	return strings.Join(parts, " ")
}

func agentTurnDebugLine(turn AgentTurnResponse) string {
	parts := compactParts(turn.Status, turn.ActionType, turn.ToolName, turn.Message)
	line := strings.Join(parts, " ")
	if turn.ResponseFormat != "" {
		line += "\n  response_format: " + turn.ResponseFormat
	}
	if turn.ProtocolErrorCode != "" {
		line += "\n  protocol_error_code: " + turn.ProtocolErrorCode
	}
	if turn.CorrectionMessage != "" {
		line += "\n  correction_message: " + turn.CorrectionMessage
	}
	if len(turn.RequestMessages) > 0 {
		line += "\n  request_messages:"
		for index, message := range turn.RequestMessages {
			label := message.Role
			if message.Kind != "" {
				label += "/" + message.Kind
			}
			line += fmt.Sprintf("\n    %d. %s: %s", index+1, label, message.Content)
		}
	}
	if turn.RawResponse != "" {
		line += "\n  raw_response: " + turn.RawResponse
	}
	if turn.ResponsePreview != "" {
		line += "\n  response_preview: " + turn.ResponsePreview
	}
	if line == "" {
		return "turn"
	}

	return line
}

func toolCallStatus(call ToolCallResponse) string {
	if call.IsError {
		return "error"
	}

	return "ok"
}

func writeArguments(buffer *bytes.Buffer, arguments map[string]string) {
	if len(arguments) == 0 {
		return
	}
	keys := make([]string, 0, len(arguments))
	for key := range arguments {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(buffer, "  %s: %s\n", key, arguments[key])
	}
}

func writeSectionHeader(buffer *bytes.Buffer, title string) {
	buffer.WriteString("\n")
	buffer.WriteString(title)
	buffer.WriteString("\n")
}

func compactParts(parts ...string) []string {
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			result = append(result, part)
		}
	}

	return result
}
