package agent

import (
	"strconv"
	"strings"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

func buildInitialTurns(task run.TaskSpec) []outbound.ModelTurn {
	parts := []outbound.ModelPart{
		{Kind: outbound.ModelPartKindTaskPrompt, Text: task.Prompt},
	}
	if workspaceContext := buildWorkspaceContextPart(task); workspaceContext != "" {
		parts = append(parts, outbound.ModelPart{
			Kind: outbound.ModelPartKindWorkspaceContext,
			Text: workspaceContext,
		})
	}

	return []outbound.ModelTurn{{Role: outbound.ModelRoleUser, Parts: parts}}
}

func assistantTurn(content string) outbound.ModelTurn {
	return outbound.ModelTurn{
		Role: outbound.ModelRoleAssistant,
		Parts: []outbound.ModelPart{
			{Kind: outbound.ModelPartKindAssistantResponse, Text: content},
		},
	}
}

func assistantAttemptTurn(content string) outbound.ModelTurn {
	return outbound.ModelTurn{
		Role: outbound.ModelRoleAssistant,
		Parts: []outbound.ModelPart{
			{Kind: outbound.ModelPartKindAssistantAttempt, Text: boundedAssistantAttempt(content)},
		},
	}
}

func assistantToolCallTurn(content string, calls []outbound.ModelToolCall) outbound.ModelTurn {
	parts := []outbound.ModelPart{}
	if text := strings.TrimSpace(content); text != "" {
		parts = append(parts, outbound.ModelPart{
			Kind: outbound.ModelPartKindAssistantResponse,
			Text: text,
		})
	}
	for _, call := range calls {
		parts = append(parts, outbound.ModelPart{
			Kind:          outbound.ModelPartKindToolCall,
			ToolCallID:    call.ID,
			ToolName:      call.Name,
			ToolArguments: copyArguments(call.Arguments),
		})
	}

	return outbound.ModelTurn{
		Role:  outbound.ModelRoleAssistant,
		Parts: parts,
	}
}

func runtimeTurn(kind outbound.ModelPartKind, content string) outbound.ModelTurn {
	return outbound.ModelTurn{
		Role: outbound.ModelRoleRuntime,
		Parts: []outbound.ModelPart{
			{Kind: kind, Text: content},
		},
	}
}

func toolResultTurn(results []outbound.ModelPart) outbound.ModelTurn {
	return outbound.ModelTurn{
		Role:  outbound.ModelRoleTool,
		Parts: results,
	}
}

func buildWorkspaceContextPart(task run.TaskSpec) string {
	if len(task.Attachments) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("Authorized input sources available through workspace:\n")
	for index, attachment := range task.Attachments {
		builder.WriteString("- source_id: ")
		builder.WriteString(workspaceSourceID(index))
		builder.WriteString("; path: ")
		builder.WriteString(attachment.Filename)
		builder.WriteString("; target_path_after_stage: /workspace/")
		builder.WriteString(attachment.Filename)
		if attachment.MediaType != "" {
			builder.WriteString("; media_type: ")
			builder.WriteString(attachment.MediaType)
		}
		builder.WriteString("; size_bytes: ")
		builder.WriteString(strconv.FormatInt(attachmentSizeBytes(attachment), 10))
		builder.WriteString("\n")
	}
	builder.WriteString("These are source metadata only; their file contents are not available in /workspace until staged.\n")
	if len(task.Attachments) == 1 {
		builder.WriteString("If the user refers to this file without naming it, call workspace stage with the source_id above before using file contents.\n")
	}

	return builder.String()
}

func workspaceSourceID(index int) string {
	id := strconv.Itoa(index + 1)
	if len(id) >= 3 {
		return "input_" + id
	}

	return "input_" + strings.Repeat("0", 3-len(id)) + id
}

func attachmentSizeBytes(attachment run.TaskAttachment) int64 {
	if attachment.SizeBytes > 0 {
		return attachment.SizeBytes
	}

	return int64(len(attachment.Content))
}
