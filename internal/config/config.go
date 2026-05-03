package config

import (
	"os"
	"time"
)

const defaultHTTPAddr = ":8080"
const defaultModelTimeout = 30 * time.Second

// ModelProvider identifies the configured model provider.
type ModelProvider string

const (
	// ModelProviderNoop uses a placeholder local model implementation.
	ModelProviderNoop ModelProvider = "noop"
	// ModelProviderOpenAICompatible uses an OpenAI-compatible chat completions API.
	ModelProviderOpenAICompatible ModelProvider = "openai_compatible"
	// ModelProviderOpenAI uses the official OpenAI API.
	ModelProviderOpenAI ModelProvider = "openai"
	// ModelProviderAnthropic uses the Anthropic Messages API.
	ModelProviderAnthropic ModelProvider = "anthropic"
	// ModelProviderGemini uses the Google Gemini generateContent API.
	ModelProviderGemini ModelProvider = "gemini"
)

// LLMConfig contains model provider configuration.
type LLMConfig struct {
	Provider ModelProvider
	BaseURL  string
	Model    string
	APIKey   string
	Timeout  time.Duration
}

// Config contains runtime configuration.
type Config struct {
	HTTPAddr string
	Version  string
	LLM      LLMConfig
}

// Load reads runtime configuration from the environment.
func Load(version string) Config {
	if version == "" {
		version = "dev"
	}

	addr := os.Getenv("AGENTPOOL_HTTP_ADDR")
	if addr == "" {
		addr = defaultHTTPAddr
	}

	return Config{
		HTTPAddr: addr,
		Version:  version,
		LLM:      loadLLMConfig(),
	}
}

func loadLLMConfig() LLMConfig {
	provider := ModelProvider(envOrDefault("AGENTPOOL_MODEL_PROVIDER", string(ModelProviderNoop)))

	return LLMConfig{
		Provider: provider,
		BaseURL:  envOrDefault("AGENTPOOL_MODEL_BASE_URL", defaultModelBaseURL(provider)),
		Model:    envOrDefault("AGENTPOOL_MODEL_NAME", defaultModelName(provider)),
		APIKey:   os.Getenv("AGENTPOOL_MODEL_API_KEY"),
		Timeout:  durationEnvOrDefault("AGENTPOOL_MODEL_TIMEOUT", defaultModelTimeout),
	}
}

func defaultModelBaseURL(provider ModelProvider) string {
	switch provider {
	case ModelProviderOpenAI:
		return "https://api.openai.com/v1"
	case ModelProviderAnthropic:
		return "https://api.anthropic.com"
	case ModelProviderGemini:
		return "https://generativelanguage.googleapis.com/v1beta"
	default:
		return ""
	}
}

func defaultModelName(provider ModelProvider) string {
	switch provider {
	case ModelProviderOpenAICompatible:
		return "local-model"
	case ModelProviderOpenAI:
		return "gpt-4.1-mini"
	case ModelProviderAnthropic:
		return "claude-sonnet-4-5"
	case ModelProviderGemini:
		return "gemini-2.5-flash"
	default:
		return "noop"
	}
}

func envOrDefault(name string, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}

	return value
}

func durationEnvOrDefault(name string, fallback time.Duration) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}

	return duration
}
