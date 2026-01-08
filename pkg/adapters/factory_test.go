package adapters

import (
	"testing"
)

func TestNew_Prometheus(t *testing.T) {
	config := map[string]string{
		"url":   "http://prometheus:9090",
		"query": "up",
	}

	adapter, err := New("prometheus", config, 60)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	promAdapter, ok := adapter.(*PrometheusAdapter)
	if !ok {
		t.Fatalf("expected *PrometheusAdapter, got %T", adapter)
	}

	if promAdapter.ServerURL != "http://prometheus:9090" {
		t.Errorf("ServerURL = %s, want http://prometheus:9090", promAdapter.ServerURL)
	}
	if promAdapter.Query != "up" {
		t.Errorf("Query = %s, want up", promAdapter.Query)
	}
	if promAdapter.StepSeconds != 60 {
		t.Errorf("StepSeconds = %d, want 60", promAdapter.StepSeconds)
	}
}

func TestNew_PrometheusDefaultURL(t *testing.T) {
	config := map[string]string{
		"query": "up",
	}

	adapter, err := New("prometheus", config, 60)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	promAdapter := adapter.(*PrometheusAdapter)
	if promAdapter.ServerURL != "http://localhost:9090" {
		t.Errorf("ServerURL = %s, want default http://localhost:9090", promAdapter.ServerURL)
	}
}

func TestNew_VictoriaMetrics(t *testing.T) {
	config := map[string]string{
		"url":   "http://vm:8428",
		"query": "sum(rate(requests[1m]))",
	}

	adapter, err := New("victoriametrics", config, 120)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	vmAdapter, ok := adapter.(*VictoriaMetricsAdapter)
	if !ok {
		t.Fatalf("expected *VictoriaMetricsAdapter, got %T", adapter)
	}

	if vmAdapter.ServerURL != "http://vm:8428" {
		t.Errorf("ServerURL = %s, want http://vm:8428", vmAdapter.ServerURL)
	}
	if vmAdapter.Query != "sum(rate(requests[1m]))" {
		t.Errorf("Query = %s, want sum(rate(requests[1m]))", vmAdapter.Query)
	}
	if vmAdapter.StepSeconds != 120 {
		t.Errorf("StepSeconds = %d, want 120", vmAdapter.StepSeconds)
	}
}

func TestNew_HTTP(t *testing.T) {
	config := map[string]string{
		"url":           "https://api.example.com/metrics",
		"method":        "GET",
		"valuePath":     "data.#.value",
		"timestampPath": "data.#.timestamp",
	}

	adapter, err := New("http", config, 60)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	httpAdapter, ok := adapter.(*HTTPAdapter)
	if !ok {
		t.Fatalf("expected *HTTPAdapter, got %T", adapter)
	}

	if httpAdapter.URL != "https://api.example.com/metrics" {
		t.Errorf("URL = %s, want https://api.example.com/metrics", httpAdapter.URL)
	}
	if httpAdapter.Method != "GET" {
		t.Errorf("Method = %s, want GET", httpAdapter.Method)
	}
	if httpAdapter.ValuePath != "data.#.value" {
		t.Errorf("ValuePath = %s, want data.#.value", httpAdapter.ValuePath)
	}
}

func TestNew_UnknownKind(t *testing.T) {
	config := map[string]string{}
	_, err := New("unknown", config, 60)
	if err == nil {
		t.Fatal("expected error for unknown adapter kind")
	}
}

func TestNew_PrometheusMissingQuery(t *testing.T) {
	config := map[string]string{
		"url": "http://prometheus:9090",
	}
	_, err := New("prometheus", config, 60)
	if err == nil {
		t.Fatal("expected error when query is missing")
	}
}

func TestNew_VictoriaMetricsMissingQuery(t *testing.T) {
	config := map[string]string{
		"url": "http://vm:8428",
	}
	_, err := New("victoriametrics", config, 60)
	if err == nil {
		t.Fatal("expected error when query is missing")
	}
}

func TestNew_HTTPMissingURL(t *testing.T) {
	config := map[string]string{
		"valuePath":     "data.#.value",
		"timestampPath": "data.#.timestamp",
	}
	_, err := New("http", config, 60)
	if err == nil {
		t.Fatal("expected error when url is missing")
	}
}

func TestNew_HTTPMissingPaths(t *testing.T) {
	config := map[string]string{
		"url": "https://api.example.com/metrics",
	}
	_, err := New("http", config, 60)
	if err == nil {
		t.Fatal("expected error when valuePath and timestampPath are missing")
	}
}
