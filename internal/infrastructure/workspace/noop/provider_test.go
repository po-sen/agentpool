package noop

import (
	"context"
	"errors"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/domain/run"
)

func TestProviderImplementsWorkspaceProvider(_ *testing.T) {
	var _ outbound.WorkspaceProvider = (*Provider)(nil)
}

func TestResolveWorkspaceReturnsEmptyForNone(t *testing.T) {
	provider := NewProvider()

	workspace, err := provider.ResolveWorkspace(context.Background(), outbound.WorkspaceResolveRequest{
		Source: run.WorkspaceSource{Type: run.WorkspaceSourceNone},
	})
	if err != nil {
		t.Fatalf("ResolveWorkspace() error = %v", err)
	}
	if workspace.Path != "" {
		t.Fatalf("workspace path = %q, want empty", workspace.Path)
	}
}

func TestResolveWorkspaceTreatsEmptySourceAsNone(t *testing.T) {
	provider := NewProvider()

	workspace, err := provider.ResolveWorkspace(context.Background(), outbound.WorkspaceResolveRequest{})
	if err != nil {
		t.Fatalf("ResolveWorkspace() error = %v", err)
	}
	if workspace.Path != "" {
		t.Fatalf("workspace path = %q, want empty", workspace.Path)
	}
}

func TestResolveWorkspaceRejectsUnsupportedSource(t *testing.T) {
	provider := NewProvider()

	_, err := provider.ResolveWorkspace(context.Background(), outbound.WorkspaceResolveRequest{
		Source: run.WorkspaceSource{Type: "configured"},
	})
	if !errors.Is(err, run.ErrUnknownWorkspaceSource) {
		t.Fatalf("ResolveWorkspace() error = %v, want %v", err, run.ErrUnknownWorkspaceSource)
	}
}
