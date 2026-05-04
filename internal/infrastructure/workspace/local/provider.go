package local

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

// Config contains local configured workspace provider settings.
type Config struct {
	ConfiguredPath string
}

// Provider resolves local workspace sources.
type Provider struct {
	configuredPath string
}

var _ outbound.WorkspaceProvider = (*Provider)(nil)

// NewProvider creates a local workspace provider.
func NewProvider(cfg Config) (*Provider, error) {
	return &Provider{configuredPath: cfg.ConfiguredPath}, nil
}

// ResolveWorkspace resolves the workspace requested by a run.
func (p *Provider) ResolveWorkspace(_ context.Context, request outbound.WorkspaceResolveRequest) (outbound.Workspace, error) {
	switch request.Source.EffectiveType() {
	case run.WorkspaceSourceNone:
		return outbound.Workspace{}, nil
	case run.WorkspaceSourceConfigured:
		return p.resolveConfigured()
	default:
		return outbound.Workspace{}, run.ErrUnknownWorkspaceSource
	}
}

func (p *Provider) resolveConfigured() (outbound.Workspace, error) {
	if p.configuredPath == "" {
		return outbound.Workspace{}, errors.New("configured workspace path is required")
	}

	workspacePath, err := filepath.Abs(p.configuredPath)
	if err != nil {
		return outbound.Workspace{}, fmt.Errorf("resolve workspace path: %w", err)
	}
	workspacePath, err = filepath.EvalSymlinks(workspacePath)
	if err != nil {
		return outbound.Workspace{}, fmt.Errorf("workspace path does not exist: %w", err)
	}

	info, err := os.Stat(workspacePath)
	if err != nil {
		return outbound.Workspace{}, fmt.Errorf("stat workspace path: %w", err)
	}
	if !info.IsDir() {
		return outbound.Workspace{}, errors.New("workspace path must be a directory")
	}

	return outbound.Workspace{Path: workspacePath}, nil
}
