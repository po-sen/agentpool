package config_test

import (
	"testing"
	"time"

	"github.com/po-sen/agentpool/internal/config"
)

func TestLoadUsesDefaults(t *testing.T) {
	t.Setenv("AGENTPOOL_HTTP_ADDR", "")
	clearModelEnv(t)

	cfg := config.Load("")
	if cfg.Version != "dev" {
		t.Fatalf("Version = %q, want dev", cfg.Version)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
	}
	if cfg.LLM.Provider != config.ModelProviderNoop {
		t.Fatalf("LLM.Provider = %q, want %q", cfg.LLM.Provider, config.ModelProviderNoop)
	}
	if cfg.LLM.Model != "noop" {
		t.Fatalf("LLM.Model = %q, want noop", cfg.LLM.Model)
	}
	if cfg.LLM.Timeout != 30*time.Second {
		t.Fatalf("LLM.Timeout = %s, want 30s", cfg.LLM.Timeout)
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

func TestLoadUsesEnvironmentLLMConfig(t *testing.T) {
	t.Setenv("AGENTPOOL_MODEL_PROVIDER", "openai_compatible")
	t.Setenv("AGENTPOOL_MODEL_BASE_URL", "http://localhost:11434/v1")
	t.Setenv("AGENTPOOL_MODEL_NAME", "qwen2.5-coder:7b")
	t.Setenv("AGENTPOOL_MODEL_API_KEY", "test-key")
	t.Setenv("AGENTPOOL_MODEL_TIMEOUT", "5s")

	cfg := config.Load("dev")
	if cfg.LLM.Provider != config.ModelProviderOpenAICompatible {
		t.Fatalf("LLM.Provider = %q, want %q", cfg.LLM.Provider, config.ModelProviderOpenAICompatible)
	}
	if cfg.LLM.BaseURL != "http://localhost:11434/v1" {
		t.Fatalf("LLM.BaseURL = %q", cfg.LLM.BaseURL)
	}
	if cfg.LLM.Model != "qwen2.5-coder:7b" {
		t.Fatalf("LLM.Model = %q", cfg.LLM.Model)
	}
	if cfg.LLM.APIKey != "test-key" {
		t.Fatalf("LLM.APIKey = %q", cfg.LLM.APIKey)
	}
	if cfg.LLM.Timeout != 5*time.Second {
		t.Fatalf("LLM.Timeout = %s, want 5s", cfg.LLM.Timeout)
	}
}

func TestLoadUsesProviderDefaults(t *testing.T) {
	tests := []struct {
		provider config.ModelProvider
		baseURL  string
		model    string
	}{
		{provider: config.ModelProviderOpenAI, baseURL: "https://api.openai.com/v1", model: "gpt-4.1-mini"},
		{provider: config.ModelProviderAnthropic, baseURL: "https://api.anthropic.com", model: "claude-sonnet-4-5"},
		{provider: config.ModelProviderGemini, baseURL: "https://generativelanguage.googleapis.com/v1beta", model: "gemini-2.5-flash"},
	}

	for _, tt := range tests {
		t.Run(string(tt.provider), func(t *testing.T) {
			t.Setenv("AGENTPOOL_MODEL_PROVIDER", string(tt.provider))
			t.Setenv("AGENTPOOL_MODEL_BASE_URL", "")
			t.Setenv("AGENTPOOL_MODEL_NAME", "")

			cfg := config.Load("dev")
			if cfg.LLM.BaseURL != tt.baseURL {
				t.Fatalf("LLM.BaseURL = %q, want %q", cfg.LLM.BaseURL, tt.baseURL)
			}
			if cfg.LLM.Model != tt.model {
				t.Fatalf("LLM.Model = %q, want %q", cfg.LLM.Model, tt.model)
			}
		})
	}
}

func TestLoadFallsBackForInvalidModelTimeout(t *testing.T) {
	t.Setenv("AGENTPOOL_MODEL_TIMEOUT", "bad")

	cfg := config.Load("dev")
	if cfg.LLM.Timeout != 30*time.Second {
		t.Fatalf("LLM.Timeout = %s, want 30s", cfg.LLM.Timeout)
	}
}

func clearModelEnv(t *testing.T) {
	t.Helper()

	for _, name := range []string{
		"AGENTPOOL_MODEL_PROVIDER",
		"AGENTPOOL_MODEL_BASE_URL",
		"AGENTPOOL_MODEL_NAME",
		"AGENTPOOL_MODEL_API_KEY",
		"AGENTPOOL_MODEL_TIMEOUT",
	} {
		t.Setenv(name, "")
	}
}
