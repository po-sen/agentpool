package bootstrap_test

import (
	"io"
	"testing"

	"github.com/po-sen/agentpool/internal/bootstrap"
)

func TestNewWiresVersion(t *testing.T) {
	app := bootstrap.New("test-version", io.Discard)

	if got, want := app.Version(), "agentpool test-version"; got != want {
		t.Fatalf("Version() = %q, want %q", got, want)
	}
}
