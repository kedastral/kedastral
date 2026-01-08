package config

import (
	"flag"
	"os"
	"testing"
	"time"
)

func TestGetEnv(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue string
		envValue     string
		want         string
	}{
		{
			name:         "environment variable set",
			key:          "TEST_VAR",
			defaultValue: "default",
			envValue:     "from-env",
			want:         "from-env",
		},
		{
			name:         "environment variable not set",
			key:          "NONEXISTENT_VAR",
			defaultValue: "default",
			envValue:     "",
			want:         "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			}

			got := getEnv(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getEnv() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetEnvInt(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue int
		envValue     string
		want         int
	}{
		{
			name:         "valid integer",
			key:          "TEST_INT",
			defaultValue: 10,
			envValue:     "42",
			want:         42,
		},
		{
			name:         "invalid integer",
			key:          "TEST_INT",
			defaultValue: 10,
			envValue:     "not-a-number",
			want:         10,
		},
		{
			name:         "not set",
			key:          "NONEXISTENT_INT",
			defaultValue: 99,
			envValue:     "",
			want:         99,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			}

			got := getEnvInt(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getEnvInt() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestGetEnvFloat(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue float64
		envValue     string
		want         float64
	}{
		{
			name:         "valid float",
			key:          "TEST_FLOAT",
			defaultValue: 1.0,
			envValue:     "3.14",
			want:         3.14,
		},
		{
			name:         "invalid float",
			key:          "TEST_FLOAT",
			defaultValue: 2.5,
			envValue:     "not-a-float",
			want:         2.5,
		},
		{
			name:         "not set",
			key:          "NONEXISTENT_FLOAT",
			defaultValue: 9.99,
			envValue:     "",
			want:         9.99,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			}

			got := getEnvFloat(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getEnvFloat() = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestGetEnvDuration(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue time.Duration
		envValue     string
		want         time.Duration
	}{
		{
			name:         "valid duration",
			key:          "TEST_DURATION",
			defaultValue: 1 * time.Minute,
			envValue:     "5m",
			want:         5 * time.Minute,
		},
		{
			name:         "invalid duration",
			key:          "TEST_DURATION",
			defaultValue: 30 * time.Second,
			envValue:     "not-a-duration",
			want:         30 * time.Second,
		},
		{
			name:         "not set",
			key:          "NONEXISTENT_DURATION",
			defaultValue: 10 * time.Second,
			envValue:     "",
			want:         10 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			}

			got := getEnvDuration(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getEnvDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_Defaults(t *testing.T) {
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

	os.Setenv("ADAPTER_QUERY", "sum(rate(http_requests_total[1m]))")
	defer os.Unsetenv("ADAPTER_QUERY")

	os.Args = []string{
		"cmd",
		"-workload=test-api",
		"-metric=http_rps",
		"-adapter=prometheus",
	}

	cfg := ParseFlags()

	// Check defaults
	if cfg.Listen != ":8081" {
		t.Errorf("Listen = %q, want %q", cfg.Listen, ":8081")
	}
	if cfg.Horizon != 30*time.Minute {
		t.Errorf("Horizon = %v, want 30m", cfg.Horizon)
	}
	if cfg.Step != 1*time.Minute {
		t.Errorf("Step = %v, want 1m", cfg.Step)
	}
	if cfg.TargetPerPod != 100.0 {
		t.Errorf("TargetPerPod = %f, want 100.0", cfg.TargetPerPod)
	}
	if cfg.Headroom != 1.2 {
		t.Errorf("Headroom = %f, want 1.2", cfg.Headroom)
	}
	if cfg.MinReplicas != 1 {
		t.Errorf("MinReplicas = %d, want 1", cfg.MinReplicas)
	}
	if cfg.MaxReplicas != 100 {
		t.Errorf("MaxReplicas = %d, want 100", cfg.MaxReplicas)
	}
	if cfg.Interval != 30*time.Second {
		t.Errorf("Interval = %v, want 30s", cfg.Interval)
	}
	if cfg.Window != 30*time.Minute {
		t.Errorf("Window = %v, want 30m", cfg.Window)
	}
	if cfg.LogFormat != "text" {
		t.Errorf("LogFormat = %q, want %q", cfg.LogFormat, "text")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
}

func TestConfig_CustomValues(t *testing.T) {
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

	os.Setenv("ADAPTER_QUERY", "custom_query")
	defer os.Unsetenv("ADAPTER_QUERY")

	os.Args = []string{
		"cmd",
		"-workload=my-api",
		"-metric=custom_metric",
		"-adapter=prometheus",
		"-listen=:9090",
		"-horizon=1h",
		"-step=5m",
		"-target-per-pod=200",
		"-headroom=1.5",
		"-min=2",
		"-max=50",
		"-interval=1m",
		"-window=1h",
		"-log-format=json",
		"-log-level=debug",
	}

	cfg := ParseFlags()

	if cfg.Listen != ":9090" {
		t.Errorf("Listen = %q, want %q", cfg.Listen, ":9090")
	}
	if cfg.Workload != "my-api" {
		t.Errorf("Workload = %q, want %q", cfg.Workload, "my-api")
	}
	if cfg.Metric != "custom_metric" {
		t.Errorf("Metric = %q, want %q", cfg.Metric, "custom_metric")
	}
	if cfg.Horizon != 1*time.Hour {
		t.Errorf("Horizon = %v, want 1h", cfg.Horizon)
	}
	if cfg.Step != 5*time.Minute {
		t.Errorf("Step = %v, want 5m", cfg.Step)
	}
	if cfg.TargetPerPod != 200.0 {
		t.Errorf("TargetPerPod = %f, want 200.0", cfg.TargetPerPod)
	}
	if cfg.LogFormat != "json" {
		t.Errorf("LogFormat = %q, want %q", cfg.LogFormat, "json")
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
}
