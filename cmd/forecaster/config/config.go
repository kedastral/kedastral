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
//   - TLS configuration (cert, key, CA files)
//
// Multi-workload mode is enabled via --config-file flag pointing to a YAML file.
// Single-workload mode (legacy) uses individual flags for backward compatibility.
//
// Supported configuration sources (in order of precedence):
//  1. Command-line flags
//  2. Environment variables
//  3. Default values
//
// Example usage:
//
//	cfg := config.ParseFlags()
//	workloads, err := config.LoadWorkloads(ctx, cfg)
//	// workloads contains validated workload configurations
package config

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/HatiCode/kedastral/pkg/tls"
)

// Config holds all forecaster configuration.
type Config struct {
	Listen    string
	LogFormat string
	LogLevel      string
	Storage       string
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	RedisTTL      time.Duration
	TLS           tls.Config

	Workload              string
	Metric                string
	Adapter               string
	AdapterConfig         map[string]string
	Horizon               time.Duration
	Step                  time.Duration
	TargetPerPod          float64
	Headroom              float64
	QuantileLevel         string
	MinReplicas           int
	MaxReplicas           int
	UpMaxFactorPerStep    float64
	DownMaxPercentPerStep int
	Interval              time.Duration
	Window                time.Duration
	Model                 string
	ARIMA_P               int
	ARIMA_D               int
	ARIMA_Q               int
	SARIMA_P              int
	SARIMA_D              int
	SARIMA_Q              int
	SARIMA_SP             int
	SARIMA_SD             int
	SARIMA_SQ             int
	SARIMA_S              int
	BYOMURL               string
}

// WorkloadConfig holds configuration for a single workload.
// This struct is now only populated from flags/env vars for single-workload mode.
type WorkloadConfig struct {
	Name                  string
	Metric                string
	Adapter               string
	AdapterConfig         map[string]string
	Horizon               time.Duration
	Step                  time.Duration
	Interval              time.Duration
	Window                time.Duration
	Model                 string
	TargetPerPod          float64
	Headroom              float64
	QuantileLevel         string
	MinReplicas           int
	MaxReplicas           int
	UpMaxFactorPerStep    float64
	DownMaxPercentPerStep int
	ARIMA_P               int
	ARIMA_D               int
	ARIMA_Q               int
	SARIMA_P              int
	SARIMA_D              int
	SARIMA_Q              int
	SARIMA_SP             int
	SARIMA_SD             int
	SARIMA_SQ             int
	SARIMA_S              int
	BYOMURL               string
}

// ParseFlags parses command-line flags and environment variables into a Config.
// Environment variables are used as fallbacks when flags are not provided.
// Each forecaster instance manages a single workload for security and simplicity.
func ParseFlags() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.Listen, "listen", getEnv("LISTEN", ":8081"), "HTTP listen address")

	flag.StringVar(&cfg.LogFormat, "log-format", getEnv("LOG_FORMAT", "text"), "Log format: text or json")
	flag.StringVar(&cfg.LogLevel, "log-level", getEnv("LOG_LEVEL", "info"), "Log level: debug, info, warn, error")

	flag.StringVar(&cfg.Storage, "storage", getEnv("STORAGE", "memory"), "Storage backend: memory or redis")
	flag.StringVar(&cfg.RedisAddr, "redis-addr", getEnv("REDIS_ADDR", "localhost:6379"), "Redis server address")
	flag.StringVar(&cfg.RedisPassword, "redis-password", getEnv("REDIS_PASSWORD", ""), "Redis password")
	flag.IntVar(&cfg.RedisDB, "redis-db", getEnvInt("REDIS_DB", 0), "Redis database number")
	flag.DurationVar(&cfg.RedisTTL, "redis-ttl", getEnvDuration("REDIS_TTL", 30*time.Minute), "Redis snapshot TTL")

	flag.BoolVar(&cfg.TLS.Enabled, "tls-enabled", getEnvBool("TLS_ENABLED", false), "Enable TLS for HTTP server")
	flag.StringVar(&cfg.TLS.CertFile, "tls-cert-file", getEnv("TLS_CERT_FILE", ""), "TLS certificate file")
	flag.StringVar(&cfg.TLS.KeyFile, "tls-key-file", getEnv("TLS_KEY_FILE", ""), "TLS private key file")
	flag.StringVar(&cfg.TLS.CAFile, "tls-ca-file", getEnv("TLS_CA_FILE", ""), "TLS CA certificate file for client verification")

	flag.StringVar(&cfg.Workload, "workload", getEnv("WORKLOAD", ""), "Workload name (required in single-workload mode)")
	flag.StringVar(&cfg.Metric, "metric", getEnv("METRIC", ""), "Metric name (required in single-workload mode)")
	flag.StringVar(&cfg.Adapter, "adapter", getEnv("ADAPTER", ""), "Adapter type: prometheus, victoriametrics, or http")
	flag.DurationVar(&cfg.Horizon, "horizon", getEnvDuration("HORIZON", 30*time.Minute), "Forecast horizon")
	flag.DurationVar(&cfg.Step, "step", getEnvDuration("STEP", 1*time.Minute), "Forecast step size")
	flag.Float64Var(&cfg.TargetPerPod, "target-per-pod", getEnvFloat("TARGET_PER_POD", 100.0), "Target metric value per pod")
	flag.Float64Var(&cfg.Headroom, "headroom", getEnvFloat("HEADROOM", 1.2), "Headroom multiplier (fallback when quantiles unavailable)")
	flag.StringVar(&cfg.QuantileLevel, "quantile-level", getEnv("QUANTILE_LEVEL", "0"), "Quantile level for capacity planning (p90, p95, or 0.90, 0.95). Set to 0 to disable and use headroom.")
	flag.IntVar(&cfg.MinReplicas, "min", getEnvInt("MIN_REPLICAS", 1), "Minimum replicas")
	flag.IntVar(&cfg.MaxReplicas, "max", getEnvInt("MAX_REPLICAS", 100), "Maximum replicas")
	flag.Float64Var(&cfg.UpMaxFactorPerStep, "up-max-factor", getEnvFloat("UP_MAX_FACTOR", 2.0), "Max scale-up factor per step")
	flag.IntVar(&cfg.DownMaxPercentPerStep, "down-max-percent", getEnvInt("DOWN_MAX_PERCENT", 50), "Max scale-down percent per step")
	flag.DurationVar(&cfg.Interval, "interval", getEnvDuration("INTERVAL", 30*time.Second), "Forecast interval")
	flag.DurationVar(&cfg.Window, "window", getEnvDuration("WINDOW", 30*time.Minute), "Historical window")
	flag.StringVar(&cfg.Model, "model", getEnv("MODEL", "baseline"), "Forecasting model: baseline, arima, sarima, or byom")
	flag.IntVar(&cfg.ARIMA_P, "arima-p", getEnvInt("ARIMA_P", 0), "ARIMA AR order (0=auto, default 1)")
	flag.IntVar(&cfg.ARIMA_D, "arima-d", getEnvInt("ARIMA_D", 0), "ARIMA differencing order (0=auto, default 1)")
	flag.IntVar(&cfg.ARIMA_Q, "arima-q", getEnvInt("ARIMA_Q", 0), "ARIMA MA order (0=auto, default 1)")
	flag.IntVar(&cfg.SARIMA_P, "sarima-p", getEnvInt("SARIMA_P", 0), "SARIMA non-seasonal AR order (0=auto, default 1)")
	flag.IntVar(&cfg.SARIMA_D, "sarima-d", getEnvInt("SARIMA_D", 0), "SARIMA non-seasonal differencing order (0=auto, default 1)")
	flag.IntVar(&cfg.SARIMA_Q, "sarima-q", getEnvInt("SARIMA_Q", 0), "SARIMA non-seasonal MA order (0=auto, default 1)")
	flag.IntVar(&cfg.SARIMA_SP, "sarima-sp", getEnvInt("SARIMA_SP", 1), "SARIMA seasonal AR order")
	flag.IntVar(&cfg.SARIMA_SD, "sarima-sd", getEnvInt("SARIMA_SD", 1), "SARIMA seasonal differencing order")
	flag.IntVar(&cfg.SARIMA_SQ, "sarima-sq", getEnvInt("SARIMA_SQ", 1), "SARIMA seasonal MA order")
	flag.IntVar(&cfg.SARIMA_S, "sarima-s", getEnvInt("SARIMA_S", 24), "SARIMA seasonal period (e.g., 24 for hourly with daily pattern)")
	flag.StringVar(&cfg.BYOMURL, "byom-url", getEnv("BYOM_URL", ""), "BYOM service URL (required when model=byom)")

	flag.Parse()

	cfg.AdapterConfig = parseAdapterConfig()

	if cfg.Workload == "" {
		fmt.Fprintln(os.Stderr, "Error: --workload is required")
		os.Exit(1)
	}
	if cfg.Metric == "" {
		fmt.Fprintln(os.Stderr, "Error: --metric is required")
		os.Exit(1)
	}
	if cfg.Adapter == "" {
		fmt.Fprintln(os.Stderr, "Error: --adapter is required")
		os.Exit(1)
	}

	return cfg
}

// parseAdapterConfig parses ADAPTER_* environment variables into a generic configuration map.
// Adapter-specific configuration is provided via environment variables with the ADAPTER_ prefix.
// For example: ADAPTER_QUERY, ADAPTER_URL, ADAPTER_VALUE_PATH
// Environment variable names are converted to camelCase for the map keys (ADAPTER_QUERY → query).
func parseAdapterConfig() map[string]string {
	config := make(map[string]string)

	for _, env := range os.Environ() {
		if len(env) > 8 && env[:8] == "ADAPTER_" {
			parts := splitEnv(env)
			if len(parts) == 2 {
				key := toLowerCamelCase(parts[0][8:])
				config[key] = parts[1]
			}
		}
	}

	return config
}

func splitEnv(env string) []string {
	for i := 0; i < len(env); i++ {
		if env[i] == '=' {
			return []string{env[:i], env[i+1:]}
		}
	}
	return []string{env}
}

func toLowerCamelCase(s string) string {
	if s == "" {
		return s
	}
	parts := []rune(s)
	result := make([]rune, 0, len(parts))
	nextUpper := false
	for i, r := range parts {
		if r == '_' {
			nextUpper = true
			continue
		}
		if i == 0 {
			result = append(result, toLower(r))
		} else if nextUpper {
			result = append(result, r)
			nextUpper = false
		} else {
			result = append(result, toLower(r))
		}
	}
	return string(result)
}

func toLower(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + 32
	}
	return r
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

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return value == "true" || value == "1"
	}
	return defaultValue
}

var workloadNameRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9_-]{0,251}[a-zA-Z0-9])?$`)

// LoadWorkloads creates a single workload configuration from flags/environment variables.
// Returns error if validation fails.
func LoadWorkloads(cfg *Config) ([]WorkloadConfig, error) {
	workload := WorkloadConfig{
		Name:                  cfg.Workload,
		Metric:                cfg.Metric,
		Adapter:               cfg.Adapter,
		AdapterConfig:         cfg.AdapterConfig,
		Horizon:               cfg.Horizon,
		Step:                  cfg.Step,
		Interval:              cfg.Interval,
		Window:                cfg.Window,
		Model:                 cfg.Model,
		TargetPerPod:          cfg.TargetPerPod,
		Headroom:              cfg.Headroom,
		QuantileLevel:         cfg.QuantileLevel,
		MinReplicas:           cfg.MinReplicas,
		MaxReplicas:           cfg.MaxReplicas,
		UpMaxFactorPerStep:    cfg.UpMaxFactorPerStep,
		DownMaxPercentPerStep: cfg.DownMaxPercentPerStep,
		ARIMA_P:               cfg.ARIMA_P,
		ARIMA_D:               cfg.ARIMA_D,
		ARIMA_Q:               cfg.ARIMA_Q,
		SARIMA_P:              cfg.SARIMA_P,
		SARIMA_D:              cfg.SARIMA_D,
		SARIMA_Q:              cfg.SARIMA_Q,
		SARIMA_SP:             cfg.SARIMA_SP,
		SARIMA_SD:             cfg.SARIMA_SD,
		SARIMA_SQ:             cfg.SARIMA_SQ,
		SARIMA_S:              cfg.SARIMA_S,
		BYOMURL:               cfg.BYOMURL,
	}

	if err := validateWorkload(&workload, 0); err != nil {
		return nil, err
	}

	return []WorkloadConfig{workload}, nil
}

func validateWorkload(w *WorkloadConfig, index int) error {
	if w.Name == "" {
		return fmt.Errorf("workload[%d]: name cannot be empty", index)
	}

	if !workloadNameRegex.MatchString(w.Name) {
		return fmt.Errorf("workload[%d]: invalid name %q (must be alphanumeric with dash/underscore, 1-253 chars)", index, w.Name)
	}

	if w.Metric == "" {
		return fmt.Errorf("workload %q: metric cannot be empty", w.Name)
	}

	if w.Adapter == "" {
		return fmt.Errorf("workload %q: adapter cannot be empty", w.Name)
	}

	if w.Horizon <= 0 {
		return fmt.Errorf("workload %q: horizon must be > 0", w.Name)
	}

	if w.Step <= 0 {
		return fmt.Errorf("workload %q: step must be > 0", w.Name)
	}

	if w.Step > w.Horizon {
		return fmt.Errorf("workload %q: step (%v) cannot exceed horizon (%v)", w.Name, w.Step, w.Horizon)
	}

	if w.Interval <= 0 {
		w.Interval = 30 * time.Second
	}

	if w.Window <= 0 {
		w.Window = 30 * time.Minute
	}

	if w.MinReplicas < 0 {
		return fmt.Errorf("workload %q: minReplicas cannot be negative", w.Name)
	}

	if w.MaxReplicas < w.MinReplicas {
		return fmt.Errorf("workload %q: maxReplicas (%d) < minReplicas (%d)", w.Name, w.MaxReplicas, w.MinReplicas)
	}

	if w.MaxReplicas == 0 {
		w.MaxReplicas = 100
	}

	if w.TargetPerPod <= 0 {
		return fmt.Errorf("workload %q: targetPerPod must be > 0", w.Name)
	}

	if w.Headroom <= 0 {
		w.Headroom = 1.0
	}

	if w.UpMaxFactorPerStep <= 0 {
		w.UpMaxFactorPerStep = 2.0
	}

	if w.DownMaxPercentPerStep < 0 || w.DownMaxPercentPerStep > 100 {
		return fmt.Errorf("workload %q: downMaxPercentPerStep must be 0-100", w.Name)
	}

	if w.Model == "" {
		w.Model = "baseline"
	}

	if w.Model != "baseline" && w.Model != "arima" && w.Model != "sarima" && w.Model != "byom" {
		return fmt.Errorf("workload %q: invalid model %q (must be baseline, arima, sarima, or byom)", w.Name, w.Model)
	}

	if w.Model == "byom" && w.BYOMURL == "" {
		return fmt.Errorf("workload %q: byomURL is required when model=byom", w.Name)
	}

	return nil
}
