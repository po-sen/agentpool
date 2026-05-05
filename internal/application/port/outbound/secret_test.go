package outbound

import (
	"context"
	"testing"
)

func TestSecretBrokerContract(t *testing.T) {
	var broker SecretBroker = contractSecretBroker{}

	bundle, err := broker.Resolve(context.Background(), SecretRequest{ProjectID: "project_test"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if bundle.Values["token"] != "redacted" {
		t.Fatalf("token = %q, want redacted", bundle.Values["token"])
	}
}

type contractSecretBroker struct{}

func (contractSecretBroker) Resolve(context.Context, SecretRequest) (SecretBundle, error) {
	return SecretBundle{Values: map[string]string{"token": "redacted"}}, nil
}
