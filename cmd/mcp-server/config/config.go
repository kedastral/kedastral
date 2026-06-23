// Package config provides configuration parsing for the MCP server.
package config

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/HatiCode/kedastral/pkg/durationx"
)

// Config holds all runtime configuration for the MCP server.
type Config struct {
	ForecasterURL string
	StaleAfter    time.Duration
	Transport     string // "stdio" or "sse"
	Listen        string // listen address for SSE transport
	BaseURL       string // public base URL for SSE transport
	LogFormat     string
	LogLevel      string
}

// ParseFlags parses command-line flags and environment variables into a Config.
// Flags take precedence over environment variables.
func ParseFlags() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.ForecasterURL, "forecaster-url", getEnv("FORECASTER_URL", "http://localhost:8081"), "Forecaster HTTP endpoint")
	durationx.Var(&cfg.StaleAfter, "stale-after", getEnvDuration("STALE_AFTER", 5*time.Minute), "Age threshold beyond which a snapshot is considered stale")
	flag.StringVar(&cfg.Transport, "transport", getEnv("MCP_TRANSPORT", "stdio"), "Transport mode: stdio or sse")
	flag.StringVar(&cfg.Listen, "listen", getEnv("MCP_LISTEN", ":8083"), "Listen address for SSE transport")
	flag.StringVar(&cfg.BaseURL, "base-url", getEnv("MCP_BASE_URL", ""), "Public base URL for SSE transport (e.g. http://kedastral-mcp:8083)")
	flag.StringVar(&cfg.LogFormat, "log-format", getEnv("LOG_FORMAT", "text"), "Log format (text|json)")
	flag.StringVar(&cfg.LogLevel, "log-level", getEnv("LOG_LEVEL", "info"), "Log level (debug|info|warn|error)")

	flag.Parse()

	if cfg.ForecasterURL == "" {
		fmt.Fprintln(os.Stderr, "Error: -forecaster-url is required")
		flag.Usage()
		os.Exit(1)
	}

	if cfg.Transport == "sse" && cfg.BaseURL == "" {
		fmt.Fprintln(os.Stderr, "Error: -base-url is required when using SSE transport")
		flag.Usage()
		os.Exit(1)
	}

	return cfg
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := durationx.Parse(value); err == nil {
			return d
		}
	}
	return defaultValue
}
