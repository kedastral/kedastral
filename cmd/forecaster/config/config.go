// Package config provides configuration parsing and management for the forecaster.
//
// It handles both command-line flags and environment variables, with flags taking
// precedence over environment variables. The Config struct contains all runtime
// configuration for the forecaster including:
//   - Workload identification (workload name, metric name)
//   - Forecast parameters (horizon, step)
//   - Capacity planning policy (target per pod, headroom, min/max replicas)
//   - Prometheus adapter settings (URL, query)
//   - Timing configuration (interval, window)
//   - Logging configuration (level, format)
//
// Required configuration values (workload, metric, prom-query) are validated
// and the program exits with status 1 if they are missing.
//
// Supported configuration sources (in order of precedence):
//  1. Command-line flags
//  2. Environment variables
//  3. Default values
//
// Example usage:
//
//	cfg := config.ParseFlags()
//	// cfg now contains validated configuration
package config

import (
	"flag"
	"fmt"
	"os"
	"time"
)

// Config holds all forecaster configuration.
type Config struct {
	Listen                string
	Workload              string
	Metric                string
	Horizon               time.Duration
	Step                  time.Duration
	TargetPerPod          float64
	Headroom              float64
	MinReplicas           int
	MaxReplicas           int
	UpMaxFactorPerStep    float64
	DownMaxPercentPerStep int
	PromURL               string
	PromQuery             string
	Interval              time.Duration
	Window                time.Duration
	LogFormat             string
	LogLevel              string
	Storage               string
	RedisAddr             string
	RedisPassword         string
	RedisDB               int
	RedisTTL              time.Duration
	Model                 string
	ARIMA_P               int
	ARIMA_D               int
	ARIMA_Q               int
}

// ParseFlags parses command-line flags and environment variables into a Config.
// Exits with status 1 if required flags (workload, metric, prom-query) are missing.
// Environment variables are used as fallbacks when flags are not provided.
func ParseFlags() *Config {
	cfg := &Config{}

	// Server
	flag.StringVar(&cfg.Listen, "listen", getEnv("LISTEN", ":8081"), "HTTP listen address")

	// Workload
	flag.StringVar(&cfg.Workload, "workload", getEnv("WORKLOAD", ""), "Workload name (required)")
	flag.StringVar(&cfg.Metric, "metric", getEnv("METRIC", ""), "Metric name (required)")

	// Forecast parameters
	flag.DurationVar(&cfg.Horizon, "horizon", getEnvDuration("HORIZON", 30*time.Minute), "Forecast horizon")
	flag.DurationVar(&cfg.Step, "step", getEnvDuration("STEP", 1*time.Minute), "Forecast step size")

	// Capacity policy
	flag.Float64Var(&cfg.TargetPerPod, "target-per-pod", getEnvFloat("TARGET_PER_POD", 100.0), "Target metric value per pod")
	flag.Float64Var(&cfg.Headroom, "headroom", getEnvFloat("HEADROOM", 1.2), "Headroom multiplier")
	flag.IntVar(&cfg.MinReplicas, "min", getEnvInt("MIN_REPLICAS", 1), "Minimum replicas")
	flag.IntVar(&cfg.MaxReplicas, "max", getEnvInt("MAX_REPLICAS", 100), "Maximum replicas")
	flag.Float64Var(&cfg.UpMaxFactorPerStep, "up-max-factor", getEnvFloat("UP_MAX_FACTOR", 2.0), "Max scale-up factor per step")
	flag.IntVar(&cfg.DownMaxPercentPerStep, "down-max-percent", getEnvInt("DOWN_MAX_PERCENT", 50), "Max scale-down percent per step")

	// Prometheus
	flag.StringVar(&cfg.PromURL, "prom-url", getEnv("PROM_URL", "http://localhost:9090"), "Prometheus URL")
	flag.StringVar(&cfg.PromQuery, "prom-query", getEnv("PROM_QUERY", ""), "Prometheus query (required)")

	// Timing
	flag.DurationVar(&cfg.Interval, "interval", getEnvDuration("INTERVAL", 30*time.Second), "Forecast interval")
	flag.DurationVar(&cfg.Window, "window", getEnvDuration("WINDOW", 30*time.Minute), "Historical window")

	// Logging
	flag.StringVar(&cfg.LogFormat, "log-format", getEnv("LOG_FORMAT", "text"), "Log format: text or json")
	flag.StringVar(&cfg.LogLevel, "log-level", getEnv("LOG_LEVEL", "info"), "Log level: debug, info, warn, error")

	// Storage backend
	flag.StringVar(&cfg.Storage, "storage", getEnv("STORAGE", "memory"), "Storage backend: memory or redis")
	flag.StringVar(&cfg.RedisAddr, "redis-addr", getEnv("REDIS_ADDR", "localhost:6379"), "Redis server address")
	flag.StringVar(&cfg.RedisPassword, "redis-password", getEnv("REDIS_PASSWORD", ""), "Redis password (optional)")
	flag.IntVar(&cfg.RedisDB, "redis-db", getEnvInt("REDIS_DB", 0), "Redis database number")
	flag.DurationVar(&cfg.RedisTTL, "redis-ttl", getEnvDuration("REDIS_TTL", 30*time.Minute), "Redis snapshot TTL")

	// Model selection
	flag.StringVar(&cfg.Model, "model", getEnv("MODEL", "baseline"), "Forecasting model: baseline or arima")
	flag.IntVar(&cfg.ARIMA_P, "arima-p", getEnvInt("ARIMA_P", 0), "ARIMA AR order (0=auto, default 1)")
	flag.IntVar(&cfg.ARIMA_D, "arima-d", getEnvInt("ARIMA_D", 0), "ARIMA differencing order (0=auto, default 1)")
	flag.IntVar(&cfg.ARIMA_Q, "arima-q", getEnvInt("ARIMA_Q", 0), "ARIMA MA order (0=auto, default 1)")

	flag.Parse()

	if cfg.Workload == "" {
		fmt.Fprintln(os.Stderr, "Error: --workload is required")
		os.Exit(1)
	}
	if cfg.Metric == "" {
		fmt.Fprintln(os.Stderr, "Error: --metric is required")
		os.Exit(1)
	}
	if cfg.PromQuery == "" {
		fmt.Fprintln(os.Stderr, "Error: --prom-query is required")
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

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var i int
		if _, err := fmt.Sscanf(value, "%d", &i); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		var f float64
		if _, err := fmt.Sscanf(value, "%f", &f); err == nil {
			return f
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}
