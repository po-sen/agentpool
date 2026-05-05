package shell

import (
	"fmt"
	"strings"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func formatCommandResult(result outbound.SandboxCommandResult) string {
	var output strings.Builder
	_, _ = fmt.Fprintf(&output, "exit_code: %d\n", result.ExitCode)
	if result.TimedOut {
		output.WriteString("timed_out: true\n")
	}
	output.WriteString("stdout:\n")
	writeBlock(&output, result.Stdout)
	output.WriteString("stderr:\n")
	writeBlock(&output, result.Stderr)

	return output.String()
}

func writeBlock(output *strings.Builder, content string) {
	output.WriteString(content)
	if !strings.HasSuffix(content, "\n") {
		output.WriteString("\n")
	}
}
