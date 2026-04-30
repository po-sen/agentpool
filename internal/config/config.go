package config

import "os"

const defaultHTTPAddr = ":8080"

// Config contains runtime configuration.
type Config struct {
	HTTPAddr string
	Version  string
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
	}
}
