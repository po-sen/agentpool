package agent

import (
	"strings"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestBuildInitialTurnsIncludesPromptAndWorkspaceContext(t *testing.T) {
	turns := buildInitialTurns(run.TaskSpec{
		Prompt: "read input",
		Attachments: []run.TaskAttachment{
			{Filename: "README.md", MediaType: "text/markdown", Content: []byte("hello")},
		},
	})

	if len(turns) != 1 {
		t.Fatalf("len(turns) = %d, want 1", len(turns))
	}
	if len(turns[0].Parts) != 2 {
		t.Fatalf("len(parts) = %d, want prompt and workspace context", len(turns[0].Parts))
	}
	if turns[0].Parts[0].Kind != outbound.ModelPartKindTaskPrompt || turns[0].Parts[0].Text != "read input" {
		t.Fatalf("prompt part = %#v, want original task prompt", turns[0].Parts[0])
	}
	for _, want := range []string{
		"Authorized input sources available through workspace:",
		"source_id: input_001",
		"target_path_after_stage: /workspace/README.md",
		"source metadata only",
		"call workspace stage with the source_id above before using file contents",
	} {
		if !strings.Contains(turns[0].Parts[1].Text, want) {
			t.Fatalf("workspace context = %q, want %q", turns[0].Parts[1].Text, want)
		}
	}
}
