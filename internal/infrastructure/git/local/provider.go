package local

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

// Config contains local workspace provider configuration.
type Config struct {
	WorkspacePath string
}

// Provider returns an existing local workspace checkout.
type Provider struct {
	workspacePath string
}

var _ outbound.GitProvider = (*Provider)(nil)

// NewProvider creates a local workspace source provider.
func NewProvider(cfg Config) (*Provider, error) {
	if strings.TrimSpace(cfg.WorkspacePath) == "" {
		return nil, errors.New("workspace path is required")
	}

	workspacePath, err := filepath.Abs(cfg.WorkspacePath)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace path: %w", err)
	}
	workspacePath, err = filepath.EvalSymlinks(workspacePath)
	if err != nil {
		return nil, fmt.Errorf("workspace path does not exist: %w", err)
	}

	info, err := os.Stat(workspacePath)
	if err != nil {
		return nil, fmt.Errorf("stat workspace path: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("workspace path must be a directory")
	}

	return &Provider{workspacePath: workspacePath}, nil
}

// Fetch returns the configured local workspace path.
func (p *Provider) Fetch(context.Context, outbound.GitFetchRequest) (outbound.GitCheckout, error) {
	return outbound.GitCheckout{Path: p.workspacePath}, nil
}
