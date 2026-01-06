package main

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/HatiCode/kedastral/cmd/forecaster/config"
	"github.com/HatiCode/kedastral/pkg/adapters"
)

func TestBuildAdapter_Prometheus(t *testing.T) {
	wc := &config.WorkloadConfig{
		Name:      "test-prom",
		PromURL:   "http://prometheus:9090",
		PromQuery: "sum(rate(http_requests_total[1m]))",
		Step:      60000000000, // 1 minute in nanoseconds
	}

	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	adapter, err := buildAdapter(wc, log)
	if err != nil {
		t.Fatalf("buildAdapter failed: %v", err)
	}

	promAdapter, ok := adapter.(*adapters.PrometheusAdapter)
	if !ok {
		t.Fatalf("expected *adapters.PrometheusAdapter, got %T", adapter)
	}

	if promAdapter.ServerURL != "http://prometheus:9090" {
		t.Errorf("expected ServerURL http://prometheus:9090, got %s", promAdapter.ServerURL)
	}

	if promAdapter.Query != "sum(rate(http_requests_total[1m]))" {
		t.Errorf("expected Query sum(rate(http_requests_total[1m])), got %s", promAdapter.Query)
	}

	if promAdapter.StepSeconds != 60 {
		t.Errorf("expected StepSeconds 60, got %d", promAdapter.StepSeconds)
	}
}

func TestBuildAdapter_VictoriaMetrics(t *testing.T) {
	wc := &config.WorkloadConfig{
		Name:                 "test-vm",
		VictoriaMetricsURL:   "http://victoria-metrics:8428",
		VictoriaMetricsQuery: "sum(queue_depth)",
		Step:                 120000000000, // 2 minutes in nanoseconds
	}

	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	adapter, err := buildAdapter(wc, log)
	if err != nil {
		t.Fatalf("buildAdapter failed: %v", err)
	}

	vmAdapter, ok := adapter.(*adapters.VictoriaMetricsAdapter)
	if !ok {
		t.Fatalf("expected *adapters.VictoriaMetricsAdapter, got %T", adapter)
	}

	if vmAdapter.ServerURL != "http://victoria-metrics:8428" {
		t.Errorf("expected ServerURL http://victoria-metrics:8428, got %s", vmAdapter.ServerURL)
	}

	if vmAdapter.Query != "sum(queue_depth)" {
		t.Errorf("expected Query sum(queue_depth), got %s", vmAdapter.Query)
	}

	if vmAdapter.StepSeconds != 120 {
		t.Errorf("expected StepSeconds 120, got %d", vmAdapter.StepSeconds)
	}
}

func TestBuildAdapter_HTTPAdapter(t *testing.T) {
	wc := &config.WorkloadConfig{
		Name: "test-http",
		AdapterConfig: &config.AdapterConfig{
			Type: "http",
			Config: map[string]interface{}{
				"url":             "https://api.example.com/metrics",
				"method":          "GET",
				"valuePath":       "data.#.value",
				"timestampPath":   "data.#.timestamp",
				"timestampFormat": "rfc3339",
				"stepSeconds":     60,
			},
		},
		Step: 60000000000, // 1 minute in nanoseconds
	}

	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	adapter, err := buildAdapter(wc, log)
	if err != nil {
		t.Fatalf("buildAdapter failed: %v", err)
	}

	httpAdapter, ok := adapter.(*adapters.HTTPAdapter)
	if !ok {
		t.Fatalf("expected *adapters.HTTPAdapter, got %T", adapter)
	}

	if httpAdapter.URL != "https://api.example.com/metrics" {
		t.Errorf("expected URL https://api.example.com/metrics, got %s", httpAdapter.URL)
	}

	if httpAdapter.Method != "GET" {
		t.Errorf("expected Method GET, got %s", httpAdapter.Method)
	}

	if httpAdapter.ValuePath != "data.#.value" {
		t.Errorf("expected ValuePath data.#.value, got %s", httpAdapter.ValuePath)
	}
}

func TestBuildAdapter_NoDataSource(t *testing.T) {
	wc := &config.WorkloadConfig{
		Name: "test-none",
		Step: 60000000000,
	}

	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	_, err := buildAdapter(wc, log)
	if err == nil {
		t.Fatal("expected error for workload with no data source, got nil")
	}
}

func TestLoadWorkloads_MultipleAdapterTypes(t *testing.T) {
	// Create temporary config file
	tmpFile, err := os.CreateTemp("", "workloads-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	configContent := `workloads:
  - name: prom-workload
    metric: http_rps
    prometheusURL: http://prometheus:9090
    prometheusQuery: sum(rate(http_requests_total[1m]))
    horizon: 30m
    step: 1m
    interval: 30s
    window: 1h
    model: baseline
    targetPerPod: 100
    headroom: 1.2
    minReplicas: 2
    maxReplicas: 50
    upMaxFactorPerStep: 2.0
    downMaxPercentPerStep: 50

  - name: vm-workload
    metric: queue_depth
    victoriaMetricsURL: http://victoria-metrics:8428
    victoriaMetricsQuery: sum(queue_messages)
    horizon: 1h
    step: 2m
    interval: 1m
    window: 2h
    model: baseline
    targetPerPod: 50
    headroom: 1.5
    minReplicas: 1
    maxReplicas: 20
    upMaxFactorPerStep: 3.0
    downMaxPercentPerStep: 30
`

	if _, err := tmpFile.Write([]byte(configContent)); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}
	tmpFile.Close()

	cfg := &config.Config{
		ConfigFile: tmpFile.Name(),
	}

	workloads, err := config.LoadWorkloads(context.Background(), cfg)
	if err != nil {
		t.Fatalf("LoadWorkloads failed: %v", err)
	}

	if len(workloads) != 2 {
		t.Fatalf("expected 2 workloads, got %d", len(workloads))
	}

	// Verify first workload uses Prometheus
	if workloads[0].PromQuery == "" {
		t.Error("expected first workload to have PromQuery")
	}
	if workloads[0].VictoriaMetricsQuery != "" {
		t.Error("expected first workload to not have VictoriaMetricsQuery")
	}

	// Verify second workload uses VictoriaMetrics
	if workloads[1].VictoriaMetricsQuery == "" {
		t.Error("expected second workload to have VictoriaMetricsQuery")
	}
	if workloads[1].PromQuery != "" {
		t.Error("expected second workload to not have PromQuery")
	}

	// Test adapter building for both
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	adapter1, err := buildAdapter(&workloads[0], log)
	if err != nil {
		t.Fatalf("failed to build adapter for workload 1: %v", err)
	}
	if _, ok := adapter1.(*adapters.PrometheusAdapter); !ok {
		t.Errorf("expected PrometheusAdapter for workload 1, got %T", adapter1)
	}

	adapter2, err := buildAdapter(&workloads[1], log)
	if err != nil {
		t.Fatalf("failed to build adapter for workload 2: %v", err)
	}
	if _, ok := adapter2.(*adapters.VictoriaMetricsAdapter); !ok {
		t.Errorf("expected VictoriaMetricsAdapter for workload 2, got %T", adapter2)
	}
}
