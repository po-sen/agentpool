package noop_test

import (
	"context"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
	secretnoop "github.com/po-sen/agentpool/internal/infrastructure/secret/noop"
)

func TestBrokerResolveReturnsEmptyBundle(t *testing.T) {
	bundle, err := secretnoop.NewBroker().Resolve(context.Background(), outbound.SecretRequest{
		ProjectID: "project_test",
		Names:     []string{"TOKEN"},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if bundle.Values == nil {
		t.Fatal("bundle.Values is nil")
	}
	if len(bundle.Values) != 0 {
		t.Fatalf("len(bundle.Values) = %d, want 0", len(bundle.Values))
	}
}
