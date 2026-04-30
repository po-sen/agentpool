package config_test

import (
	"testing"

	"github.com/po-sen/agentpool/internal/config"
)

func TestLoadUsesDefaults(t *testing.T) {
	t.Setenv("AGENTPOOL_HTTP_ADDR", "")

	cfg := config.Load("")
	if cfg.Version != "dev" {
		t.Fatalf("Version = %q, want dev", cfg.Version)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
	}
}

func TestLoadUsesEnvironmentHTTPAddrAndVersion(t *testing.T) {
	t.Setenv("AGENTPOOL_HTTP_ADDR", "127.0.0.1:9000")

	cfg := config.Load("v1.2.3")
	if cfg.Version != "v1.2.3" {
		t.Fatalf("Version = %q, want v1.2.3", cfg.Version)
	}
	if cfg.HTTPAddr != "127.0.0.1:9000" {
		t.Fatalf("HTTPAddr = %q, want 127.0.0.1:9000", cfg.HTTPAddr)
	}
}
