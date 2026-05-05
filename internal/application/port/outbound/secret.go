package outbound

import "context"

// SecretRequest describes a request for runtime secrets.
type SecretRequest struct {
	ProjectID string
	Names     []string
}

// SecretBundle contains resolved runtime secrets.
type SecretBundle struct {
	Values map[string]string
}

// SecretBroker resolves runtime secrets.
type SecretBroker interface {
	Resolve(context.Context, SecretRequest) (SecretBundle, error)
}
