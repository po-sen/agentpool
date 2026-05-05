package config

import (
	"testing"
	"time"
)

func TestLoadUsesDefaults(t *testing.T) {
	t.Setenv("AGENTPOOL_HTTP_ADDR", "")
	t.Setenv("AGENTPOOL_AGENT_MAX_TURNS", "")
	clearModelEnv(t)
	clearSandboxEnv(t)

	cfg := Load("")
	if cfg.Version != "dev" {
		t.Fatalf("Version = %q, want dev", cfg.Version)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
	}
	if cfg.LLM.Provider != ModelProviderNoop {
		t.Fatalf("LLM.Provider = %q, want %q", cfg.LLM.Provider, ModelProviderNoop)
	}
	if cfg.LLM.Model != "noop" {
		t.Fatalf("LLM.Model = %q, want noop", cfg.LLM.Model)
	}
	if cfg.LLM.Timeout != 30*time.Second {
		t.Fatalf("LLM.Timeout = %s, want 30s", cfg.LLM.Timeout)
	}
	if cfg.Agent.MaxTurns != 8 {
		t.Fatalf("Agent.MaxTurns = %d, want 8", cfg.Agent.MaxTurns)
	}
	if cfg.Sandbox.Provider != SandboxProviderNoop {
		t.Fatalf("Sandbox.Provider = %q, want %q", cfg.Sandbox.Provider, SandboxProviderNoop)
	}
	if cfg.Sandbox.Image != "alpine:3.20" {
		t.Fatalf("Sandbox.Image = %q, want alpine:3.20", cfg.Sandbox.Image)
	}
}

func TestLoadUsesEnvironmentHTTPAddrAndVersion(t *testing.T) {
	t.Setenv("AGENTPOOL_HTTP_ADDR", "127.0.0.1:9000")

	cfg := Load("v1.2.3")
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

	cfg := Load("dev")
	if cfg.LLM.Provider != ModelProviderOpenAICompatible {
		t.Fatalf("LLM.Provider = %q, want %q", cfg.LLM.Provider, ModelProviderOpenAICompatible)
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

func TestLoadUsesEnvironmentAgentConfig(t *testing.T) {
	t.Setenv("AGENTPOOL_AGENT_MAX_TURNS", "7")

	cfg := Load("dev")
	if cfg.Agent.MaxTurns != 7 {
		t.Fatalf("Agent.MaxTurns = %d, want 7", cfg.Agent.MaxTurns)
	}
}

func TestLoadUsesEnvironmentSandboxConfig(t *testing.T) {
	t.Setenv("AGENTPOOL_SANDBOX_PROVIDER", "docker")
	t.Setenv("AGENTPOOL_SANDBOX_IMAGE", "busybox:1.36")

	cfg := Load("dev")
	if cfg.Sandbox.Provider != SandboxProviderDocker {
		t.Fatalf("Sandbox.Provider = %q, want %q", cfg.Sandbox.Provider, SandboxProviderDocker)
	}
	if cfg.Sandbox.Image != "busybox:1.36" {
		t.Fatalf("Sandbox.Image = %q, want busybox:1.36", cfg.Sandbox.Image)
	}
}

func TestLoadFallsBackForInvalidAgentMaxTurns(t *testing.T) {
	for _, value := range []string{"bad", "0", "-1"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("AGENTPOOL_AGENT_MAX_TURNS", value)

			cfg := Load("dev")
			if cfg.Agent.MaxTurns != 8 {
				t.Fatalf("Agent.MaxTurns = %d, want 8", cfg.Agent.MaxTurns)
			}
		})
	}
}

func TestLoadUsesProviderDefaults(t *testing.T) {
	tests := []struct {
		provider ModelProvider
		baseURL  string
		model    string
	}{
		{provider: ModelProviderOpenAI, baseURL: "https://api.openai.com/v1", model: "gpt-4.1-mini"},
		{provider: ModelProviderAnthropic, baseURL: "https://api.anthropic.com", model: "claude-sonnet-4-5"},
		{provider: ModelProviderGemini, baseURL: "https://generativelanguage.googleapis.com/v1beta", model: "gemini-2.5-flash"},
	}

	for _, tt := range tests {
		t.Run(string(tt.provider), func(t *testing.T) {
			t.Setenv("AGENTPOOL_MODEL_PROVIDER", string(tt.provider))
			t.Setenv("AGENTPOOL_MODEL_BASE_URL", "")
			t.Setenv("AGENTPOOL_MODEL_NAME", "")

			cfg := Load("dev")
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

	cfg := Load("dev")
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

func clearSandboxEnv(t *testing.T) {
	t.Helper()

	for _, name := range []string{
		"AGENTPOOL_SANDBOX_PROVIDER",
		"AGENTPOOL_SANDBOX_IMAGE",
	} {
		t.Setenv(name, "")
	}
}
