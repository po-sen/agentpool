package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

const (
	defaultImage = "alpine:3.20"

	dockerBinary = "docker"
	sandboxID    = "docker-dev"
)

// Config controls the dev Docker sandbox provider.
type Config struct {
	Image string
}

// Provider runs commands through Docker CLI for local dev sandbox verification.
type Provider struct {
	image    string
	executor commandExecutor
}

var _ outbound.SandboxProvider = (*Provider)(nil)
var _ outbound.SandboxCapabilityProvider = (*Provider)(nil)
var _ outbound.SandboxCommandRunner = (*Provider)(nil)

// NewProvider creates a dev Docker sandbox provider.
func NewProvider(cfg Config) *Provider {
	return newProviderWithExecutor(cfg, osCommandExecutor{})
}

func newProviderWithExecutor(cfg Config, executor commandExecutor) *Provider {
	image := strings.TrimSpace(cfg.Image)
	if image == "" {
		image = defaultImage
	}

	return &Provider{
		image:    image,
		executor: executor,
	}
}

// Capabilities reports that Docker can run sandbox commands in dev mode.
func (p *Provider) Capabilities() outbound.SandboxCapabilities {
	return outbound.SandboxCapabilities{SupportsCommands: true}
}

// Prepare returns a command-capable dev sandbox marker.
func (p *Provider) Prepare(_ context.Context, request outbound.SandboxRequest) (outbound.Sandbox, error) {
	if err := validateWorkspace(request.Workspace); err != nil {
		return outbound.Sandbox{}, err
	}

	return outbound.Sandbox{
		ID:               sandboxID,
		SupportsCommands: true,
	}, nil
}

// Cleanup is safe and intentionally no-op because commands use docker run --rm.
func (p *Provider) Cleanup(context.Context, outbound.Sandbox) error {
	return nil
}

// RunCommand executes the requested command inside an ephemeral Docker container.
func (p *Provider) RunCommand(
	ctx context.Context,
	request outbound.SandboxCommandRequest,
) (outbound.SandboxCommandResult, error) {
	if request.Sandbox.ID == "" || !request.Sandbox.SupportsCommands {
		return outbound.SandboxCommandResult{
			Stderr:   "sandbox command execution is not available",
			ExitCode: 1,
		}, nil
	}
	if err := validateWorkspace(request.Workspace); err != nil {
		return outbound.SandboxCommandResult{}, err
	}
	if strings.TrimSpace(request.Command) == "" {
		return outbound.SandboxCommandResult{}, errors.New("command is required")
	}

	output := p.executor.Run(
		ctx,
		dockerBinary,
		dockerRunArgs(p.image, request.Workspace.RootPath, request.Command)...,
	)
	result := outbound.SandboxCommandResult{
		Stdout:   output.stdout,
		Stderr:   output.stderr,
		ExitCode: output.exitCode,
		TimedOut: output.timedOut,
	}
	if output.timedOut {
		return result, nil
	}
	if output.err != nil {
		if output.exitCode != 0 {
			return result, nil
		}

		return result, fmt.Errorf("docker CLI execution failed: %w", output.err)
	}

	return result, nil
}

func validateWorkspace(workspace outbound.Workspace) error {
	if strings.TrimSpace(workspace.RootPath) == "" {
		return errors.New("workspace root path is required")
	}

	return nil
}

func dockerRunArgs(image string, workspacePath string, command string) []string {
	return []string{
		"run",
		"--rm",
		"--network",
		"none",
		"-v",
		workspacePath + ":/workspace:rw",
		"-w",
		"/workspace",
		image,
		"/bin/sh",
		"-lc",
		command,
	}
}

type commandExecutor interface {
	Run(ctx context.Context, name string, args ...string) commandOutput
}

type commandOutput struct {
	stdout   string
	stderr   string
	exitCode int
	timedOut bool
	err      error
}

type osCommandExecutor struct{}

func (osCommandExecutor) Run(ctx context.Context, name string, args ...string) commandOutput {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := commandOutput{
		stdout: stdout.String(),
		stderr: stderr.String(),
		err:    err,
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			output.exitCode = exitErr.ExitCode()
		}
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		output.timedOut = true
		if output.exitCode == 0 {
			output.exitCode = -1
		}
	}

	return output
}
