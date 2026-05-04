package noop

import (
	"context"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestProviderFetchReturnsEmptyCheckout(t *testing.T) {
	provider := NewProvider()

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
