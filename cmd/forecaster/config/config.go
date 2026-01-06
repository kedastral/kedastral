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
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/HatiCode/kedastral/pkg/tls"
	"gopkg.in/yaml.v3"
)

// Config holds all forecaster configuration.
type Config struct {
	Listen        string
	ConfigFile    string
	LogFormat     string
	LogLevel      string
	Storage       string
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	RedisTTL      time.Duration
	TLS           tls.Config

	Workload              string
	Metric                string
	Horizon               time.Duration
	Step                  time.Duration
	TargetPerPod          float64
	Headroom              float64
	QuantileLevel         string
	MinReplicas           int
	MaxReplicas           int
	UpMaxFactorPerStep    float64
	DownMaxPercentPerStep int
	PromURL               string
	PromQuery             string
	VictoriaMetricsURL    string
	VictoriaMetricsQuery  string
	Interval              time.Duration
	Window                time.Duration
	Model                 string
	ARIMA_P               int
	ARIMA_D               int
	ARIMA_Q               int
	BYOMURL               string
}

// WorkloadConfig holds configuration for a single workload.
type WorkloadConfig struct {
	Name                  string         `yaml:"name"`
	Metric                string         `yaml:"metric"`
	PromURL               string         `yaml:"prometheusURL,omitempty"`
	PromQuery             string         `yaml:"prometheusQuery,omitempty"`
	VictoriaMetricsURL    string         `yaml:"victoriaMetricsURL,omitempty"`
	VictoriaMetricsQuery  string         `yaml:"victoriaMetricsQuery,omitempty"`
	AdapterConfig         *AdapterConfig `yaml:"adapter,omitempty"`
	Horizon               time.Duration  `yaml:"horizon"`
	Step                  time.Duration  `yaml:"step"`
	Interval              time.Duration  `yaml:"interval"`
	Window                time.Duration  `yaml:"window"`
	Model                 string         `yaml:"model"`
	TargetPerPod          float64        `yaml:"targetPerPod"`
	Headroom              float64        `yaml:"headroom"`
	QuantileLevel         string         `yaml:"quantileLevel,omitempty"`
	MinReplicas           int            `yaml:"minReplicas"`
	MaxReplicas           int            `yaml:"maxReplicas"`
	UpMaxFactorPerStep    float64        `yaml:"upMaxFactorPerStep"`
	DownMaxPercentPerStep int            `yaml:"downMaxPercentPerStep"`
	ARIMA_P               int            `yaml:"arimaP,omitempty"`
	ARIMA_D               int            `yaml:"arimaD,omitempty"`
	ARIMA_Q               int            `yaml:"arimaQ,omitempty"`
	BYOMURL               string         `yaml:"byomURL,omitempty"`
}

// AdapterConfig holds configuration for generic adapters (HTTP).
type AdapterConfig struct {
	Type   string         `yaml:"type"`
	Config map[string]any `yaml:"config"`
}

type workloadsFile struct {
	Workloads []WorkloadConfig `yaml:"workloads"`
}

// ParseFlags parses command-line flags and environment variables into a Config.
// When --config-file is provided, single-workload flags are optional (for backward compatibility).
// Environment variables are used as fallbacks when flags are not provided.
func ParseFlags() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.Listen, "listen", getEnv("LISTEN", ":8081"), "HTTP listen address")
	flag.StringVar(&cfg.ConfigFile, "config-file", getEnv("CONFIG_FILE", ""), "Path to workloads YAML config file (enables multi-workload mode)")

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
	flag.DurationVar(&cfg.Horizon, "horizon", getEnvDuration("HORIZON", 30*time.Minute), "Forecast horizon")
	flag.DurationVar(&cfg.Step, "step", getEnvDuration("STEP", 1*time.Minute), "Forecast step size")
	flag.Float64Var(&cfg.TargetPerPod, "target-per-pod", getEnvFloat("TARGET_PER_POD", 100.0), "Target metric value per pod")
	flag.Float64Var(&cfg.Headroom, "headroom", getEnvFloat("HEADROOM", 1.2), "Headroom multiplier (fallback when quantiles unavailable)")
	flag.StringVar(&cfg.QuantileLevel, "quantile-level", getEnv("QUANTILE_LEVEL", "0"), "Quantile level for capacity planning (p90, p95, or 0.90, 0.95). Set to 0 to disable and use headroom.")
	flag.IntVar(&cfg.MinReplicas, "min", getEnvInt("MIN_REPLICAS", 1), "Minimum replicas")
	flag.IntVar(&cfg.MaxReplicas, "max", getEnvInt("MAX_REPLICAS", 100), "Maximum replicas")
	flag.Float64Var(&cfg.UpMaxFactorPerStep, "up-max-factor", getEnvFloat("UP_MAX_FACTOR", 2.0), "Max scale-up factor per step")
	flag.IntVar(&cfg.DownMaxPercentPerStep, "down-max-percent", getEnvInt("DOWN_MAX_PERCENT", 50), "Max scale-down percent per step")
	flag.StringVar(&cfg.PromURL, "prom-url", getEnv("PROM_URL", "http://localhost:9090"), "Prometheus URL")
	flag.StringVar(&cfg.PromQuery, "prom-query", getEnv("PROM_QUERY", ""), "Prometheus query (single-workload mode, mutually exclusive with victoria-metrics-query)")
	flag.StringVar(&cfg.VictoriaMetricsURL, "victoria-metrics-url", getEnv("VICTORIA_METRICS_URL", "http://localhost:8428"), "VictoriaMetrics URL")
	flag.StringVar(&cfg.VictoriaMetricsQuery, "victoria-metrics-query", getEnv("VICTORIA_METRICS_QUERY", ""), "VictoriaMetrics query (single-workload mode, mutually exclusive with prom-query)")
	flag.DurationVar(&cfg.Interval, "interval", getEnvDuration("INTERVAL", 30*time.Second), "Forecast interval")
	flag.DurationVar(&cfg.Window, "window", getEnvDuration("WINDOW", 30*time.Minute), "Historical window")
	flag.StringVar(&cfg.Model, "model", getEnv("MODEL", "baseline"), "Forecasting model: baseline, arima, or byom")
	flag.IntVar(&cfg.ARIMA_P, "arima-p", getEnvInt("ARIMA_P", 0), "ARIMA AR order (0=auto, default 1)")
	flag.IntVar(&cfg.ARIMA_D, "arima-d", getEnvInt("ARIMA_D", 0), "ARIMA differencing order (0=auto, default 1)")
	flag.IntVar(&cfg.ARIMA_Q, "arima-q", getEnvInt("ARIMA_Q", 0), "ARIMA MA order (0=auto, default 1)")
	flag.StringVar(&cfg.BYOMURL, "byom-url", getEnv("BYOM_URL", ""), "BYOM service URL (required when model=byom)")

	flag.Parse()

	if cfg.ConfigFile == "" {
		if cfg.Workload == "" {
			fmt.Fprintln(os.Stderr, "Error: --workload is required (or use --config-file for multi-workload mode)")
			os.Exit(1)
		}
		if cfg.Metric == "" {
			fmt.Fprintln(os.Stderr, "Error: --metric is required (or use --config-file for multi-workload mode)")
			os.Exit(1)
		}
		promSet := cfg.PromQuery != ""
		vmSet := cfg.VictoriaMetricsQuery != ""
		if !promSet && !vmSet {
			fmt.Fprintln(os.Stderr, "Error: --prom-query or --victoria-metrics-query is required (or use --config-file for multi-workload mode)")
			os.Exit(1)
		}
		if promSet && vmSet {
			fmt.Fprintln(os.Stderr, "Error: --prom-query and --victoria-metrics-query are mutually exclusive")
			os.Exit(1)
		}
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

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return value == "true" || value == "1"
	}
	return defaultValue
}

var workloadNameRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9_-]{0,251}[a-zA-Z0-9])?$`)

// LoadWorkloads loads workload configurations from file or creates single workload from flags.
// Returns error if validation fails or if neither config file nor single-workload flags are provided.
func LoadWorkloads(ctx context.Context, cfg *Config) ([]WorkloadConfig, error) {
	if cfg.ConfigFile != "" {
		return loadWorkloadsFromFile(ctx, cfg.ConfigFile)
	}

	workload := WorkloadConfig{
		Name:                  cfg.Workload,
		Metric:                cfg.Metric,
		PromURL:               cfg.PromURL,
		PromQuery:             cfg.PromQuery,
		VictoriaMetricsURL:    cfg.VictoriaMetricsURL,
		VictoriaMetricsQuery:  cfg.VictoriaMetricsQuery,
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
		BYOMURL:               cfg.BYOMURL,
	}

	if err := validateWorkload(&workload, 0); err != nil {
		return nil, err
	}

	return []WorkloadConfig{workload}, nil
}

func loadWorkloadsFromFile(ctx context.Context, path string) ([]WorkloadConfig, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	doneCh := make(chan struct {
		workloads []WorkloadConfig
		err       error
	}, 1)

	go func() {
		data, err := os.ReadFile(path)
		if err != nil {
			doneCh <- struct {
				workloads []WorkloadConfig
				err       error
			}{nil, fmt.Errorf("read config file: %w", err)}
			return
		}

		var wf workloadsFile
		if err := yaml.Unmarshal(data, &wf); err != nil {
			doneCh <- struct {
				workloads []WorkloadConfig
				err       error
			}{nil, fmt.Errorf("parse yaml: %w", err)}
			return
		}

		if len(wf.Workloads) == 0 {
			doneCh <- struct {
				workloads []WorkloadConfig
				err       error
			}{nil, errors.New("no workloads defined in config file")}
			return
		}

		seen := make(map[string]bool)
		for i := range wf.Workloads {
			if err := validateWorkload(&wf.Workloads[i], i); err != nil {
				doneCh <- struct {
					workloads []WorkloadConfig
					err       error
				}{nil, err}
				return
			}

			if seen[wf.Workloads[i].Name] {
				doneCh <- struct {
					workloads []WorkloadConfig
					err       error
				}{nil, fmt.Errorf("workload[%d]: duplicate name %q", i, wf.Workloads[i].Name)}
				return
			}
			seen[wf.Workloads[i].Name] = true
		}

		doneCh <- struct {
			workloads []WorkloadConfig
			err       error
		}{wf.Workloads, nil}
	}()

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("load workloads timeout: %w", ctx.Err())
	case result := <-doneCh:
		return result.workloads, result.err
	}
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

	promSet := w.PromQuery != ""
	vmSet := w.VictoriaMetricsQuery != ""
	adapterSet := w.AdapterConfig != nil

	sourceCount := 0
	if promSet {
		sourceCount++
	}
	if vmSet {
		sourceCount++
	}
	if adapterSet {
		sourceCount++
	}

	if sourceCount == 0 {
		return fmt.Errorf("workload %q: must specify one of: prometheusQuery, victoriaMetricsQuery, or adapter config", w.Name)
	}

	if sourceCount > 1 {
		return fmt.Errorf("workload %q: only one data source allowed (prometheusQuery, victoriaMetricsQuery, or adapter config)", w.Name)
	}

	if promSet && w.PromURL == "" {
		w.PromURL = "http://localhost:9090"
	}

	if vmSet && w.VictoriaMetricsURL == "" {
		w.VictoriaMetricsURL = "http://localhost:8428"
	}

	if adapterSet {
		if w.AdapterConfig.Type == "" {
			return fmt.Errorf("workload %q: adapter.type cannot be empty", w.Name)
		}
		if w.AdapterConfig.Type != "http" {
			return fmt.Errorf("workload %q: unsupported adapter type %q (currently only 'http' is supported)", w.Name, w.AdapterConfig.Type)
		}
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

	if w.Model != "baseline" && w.Model != "arima" && w.Model != "byom" {
		return fmt.Errorf("workload %q: invalid model %q (must be baseline, arima, or byom)", w.Name, w.Model)
	}

	if w.Model == "byom" && w.BYOMURL == "" {
		return fmt.Errorf("workload %q: byomURL is required when model=byom", w.Name)
	}

	return nil
}
