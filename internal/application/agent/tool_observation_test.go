package agent

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestBuildToolObservationFormatsSuccess(t *testing.T) {
	got := buildToolObservation("workspace", nil, outbound.ToolResult{Content: "files:\n/workspace/input/README.md\n"})
	want := "Tool result for workspace:\nfiles:\n/workspace/input/README.md\n"
	if got != want {
		t.Fatalf("observation = %q, want %q", got, want)
	}
}

func TestBuildToolObservationTruncatesLongContent(t *testing.T) {
	got := buildToolObservation("sandbox_exec", nil, outbound.ToolResult{
		Content: strings.Repeat("界", maxToolObservationLength),
		IsError: true,
	})

	if len(got) > maxToolObservationLength {
		t.Fatalf("len(observation) = %d, want <= %d", len(got), maxToolObservationLength)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("observation is not valid UTF-8: %q", got)
	}
	if !strings.HasSuffix(got, agentTurnPreviewTruncatedMarker) {
		t.Fatalf("observation does not contain truncation marker: %q", got)
	}
}

func TestBuildToolObservationFormatsError(t *testing.T) {
	got := buildToolObservation("workspace", nil, outbound.ToolResult{Content: "path is not available", IsError: true})
	want := "Tool error for workspace:\npath is not available"
	if got != want {
		t.Fatalf("observation = %q, want %q", got, want)
	}
}

func TestBuildToolObservationHintsAfterPDFSearchError(t *testing.T) {
	got := buildToolObservation("sandbox_exec", map[string]string{
		"command": "pdftotext /workspace/input/manual.pdf - | grep exact",
	}, outbound.ToolResult{
		Content: "exit_code: 1\nstdout:\n\nstderr:\npdftotext warnings omitted: 12 line(s)\n",
		IsError: true,
	})

	for _, want := range []string{
		"Do not repeat the exact same grep.",
		"Search broader relevant terms across all PDFs",
		"not only exact prompt nouns",
		"Avoid broad one-character terms and file-title/TOC terms",
		`pdftotext with "-" for stdout`,
		"write your own small script under /workspace/work",
		"inspect nearby context before final",
		"for f in /workspace/input/*.pdf",
		`pdftotext "$f" - 2>/dev/null`,
		"term1|term2|term3",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("observation does not contain %q:\n%s", want, got)
		}
	}
}

func TestBuildToolObservationHintsAfterPDFSearchSuccess(t *testing.T) {
	got := buildToolObservation("sandbox_exec", map[string]string{
		"command": `for f in /workspace/input/*.pdf; do pdftotext "$f" - | grep -nEi 'steam|cook'; done`,
	}, outbound.ToolResult{
		Content: "exit_code: 0\nstdout:\nFILE:/workspace/input/manual.pdf\n12:steam setting\nstderr:\n",
	})

	if !strings.Contains(got, "Tool result for sandbox_exec:\nHint: These are search hits only") {
		t.Fatalf("success hint should appear before search output:\n%s", got)
	}
	for _, want := range []string{
		"These are search hits only",
		"Ignore title/TOC-only hits",
		"search again with task-specific terms",
		"inspect nearby context around substantive line numbers with a different command",
		`pdftotext '/workspace/input/manual.pdf' -`,
		`nl -ba | sed -n '512,528p'`,
		"Final answer must use the user's language",
		"do not claim a procedure exists unless context says it",
		"answer first",
		"cite file names/locations",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("observation does not contain %q:\n%s", want, got)
		}
	}
}

func TestBuildToolObservationHintsAfterSinglePDFSearchSuccess(t *testing.T) {
	got := buildToolObservation("sandbox_exec", map[string]string{
		"command": `pdftotext /workspace/input/manual.pdf - | grep -nEi 'steam|cook'`,
	}, outbound.ToolResult{
		Content: "exit_code: 0\nstdout:\n12:steam setting\nstderr:\n",
	})

	if !strings.Contains(got, "Hint: These are search hits only") {
		t.Fatalf("single PDF search should require context:\n%s", got)
	}
}

func TestBuildToolObservationHintsAfterPDFSearchScriptSuccess(t *testing.T) {
	got := buildToolObservation("sandbox_exec", map[string]string{
		"command": `/workspace/work/pdf_search.sh 'steam|cook'`,
	}, outbound.ToolResult{
		Content: "exit_code: 0\nstdout:\nFILE:/workspace/input/manual.pdf\n12:steam setting\nstderr:\n",
	})

	if !strings.Contains(got, "Hint: These are search hits only") {
		t.Fatalf("PDF search script should require context:\n%s", got)
	}
}

func TestBuildToolObservationDoesNotHintAfterEmptyPDFSearchSuccess(t *testing.T) {
	got := buildToolObservation("sandbox_exec", map[string]string{
		"command": `pdftotext /workspace/input/manual.pdf - | grep -nEi 'steam|cook'`,
	}, outbound.ToolResult{
		Content: "exit_code: 0\nstdout:\n\nstderr:\n",
	})

	if strings.Contains(got, "Hint: These are search hits only") {
		t.Fatalf("empty stdout should not be treated as search hits:\n%s", got)
	}
}

func TestBuildToolObservationHintsAfterPDFContextSuccess(t *testing.T) {
	got := buildToolObservation("sandbox_exec", map[string]string{
		"command": `pdftotext /workspace/input/manual.pdf - 2>/dev/null | sed -n '18,25p'`,
	}, outbound.ToolResult{
		Content: "exit_code: 0\nstdout:\nmanual title\nstderr:\n",
	})

	for _, want := range []string{
		"This is nearby PDF context",
		"title/TOC-only or irrelevant",
		"provided files do not contain the requested instructions",
		"Answer in the user's language",
		"中文問題請用中文回答",
		"preserve the user's exact terms",
		"cite the file name and inspected location",
		"do not tell the user to search the document",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("observation does not contain %q:\n%s", want, got)
		}
	}
}
