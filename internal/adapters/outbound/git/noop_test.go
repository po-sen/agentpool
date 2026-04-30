package git_test

import (
	"context"
	"testing"

	"github.com/po-sen/agentpool/internal/adapters/outbound/git"
	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestNoopProviderFetchReturnsEmptyCheckout(t *testing.T) {
	provider := git.NewNoopProvider()

	checkout, err := provider.Fetch(context.Background(), outbound.GitFetchRequest{
		RepositoryURL: "https://example.com/repo.git",
		Branch:        "main",
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if checkout != (outbound.GitCheckout{}) {
		t.Fatalf("checkout = %#v, want empty checkout", checkout)
	}
}
