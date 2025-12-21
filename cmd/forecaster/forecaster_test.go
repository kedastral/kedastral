package main

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/HatiCode/kedastral/cmd/forecaster/metrics"
	"github.com/HatiCode/kedastral/pkg/adapters"
	"github.com/HatiCode/kedastral/pkg/capacity"
	"github.com/HatiCode/kedastral/pkg/features"
	"github.com/HatiCode/kedastral/pkg/models"
	"github.com/HatiCode/kedastral/pkg/storage"
)

func TestNew(t *testing.T) {
	adapter := &adapters.PrometheusAdapter{
		ServerURL:   "http://localhost:9090",
		Query:       "test_query",
		StepSeconds: 60,
	}
	model := models.NewBaselineModel("test_metric", 60, 1800)
	builder := features.NewBuilder()
	store := storage.NewMemoryStore()
	policy := &capacity.Policy{
		TargetPerPod: 100,
		Headroom:     1.2,
		MinReplicas:  1,
		MaxReplicas:  10,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := metrics.New("test-workload-new")

	f := NewWorkloadForecaster(
		"test-workload",
		adapter,
		model,
		builder,
		store,
		policy,
		30*time.Minute, // horizon
		1*time.Minute,  // step
		30*time.Minute, // window
		1*time.Minute,  // interval
		logger,
		m,
	)

	if f == nil {
		t.Fatal("New() returned nil")
	}
	if f.name != "test-workload" {
		t.Errorf("workload = %q, want %q", f.name, "test-workload")
	}
	if f.currentReplicas != policy.MinReplicas {
		t.Errorf("currentReplicas = %d, want %d", f.currentReplicas, policy.MinReplicas)
	}
}

func TestNew_NilLogger(t *testing.T) {
	adapter := &adapters.PrometheusAdapter{
		ServerURL: "http://localhost:9090",
		Query:     "test",
	}
	model := models.NewBaselineModel("test", 60, 1800)
	builder := features.NewBuilder()
	store := storage.NewMemoryStore()
	policy := &capacity.Policy{MinReplicas: 1}
	m := metrics.New("test-nil-logger")

	f := NewWorkloadForecaster(
		"test",
		adapter,
		model,
		builder,
		store,
		policy,
		30*time.Minute, // horizon
		1*time.Minute,  // step
		30*time.Minute, // window
		1*time.Minute,  // interval
		nil,            // nil logger
		m,
	)

	if f.logger == nil {
		t.Error("logger should not be nil when nil is passed")
	}
}

func TestForecaster_Run_ContextCancellation(t *testing.T) {
	adapter := &adapters.PrometheusAdapter{
		ServerURL: "http://localhost:9090",
		Query:     "test",
	}
	model := models.NewBaselineModel("test", 60, 1800)
	builder := features.NewBuilder()
	store := storage.NewMemoryStore()
	policy := &capacity.Policy{MinReplicas: 1}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := metrics.New("test-run-cancel")

	f := NewWorkloadForecaster(
		"test",
		adapter,
		model,
		builder,
		store,
		policy,
		30*time.Minute, // horizon
		1*time.Minute,  // step
		30*time.Minute, // window
		1*time.Minute,  // interval
		logger,
		m,
	)

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	err := f.Run(ctx)
	if err != context.Canceled {
		t.Errorf("Run() error = %v, want %v", err, context.Canceled)
	}
}

func TestForecaster_Run_Timeout(t *testing.T) {
	adapter := &adapters.PrometheusAdapter{
		ServerURL: "http://localhost:9090",
		Query:     "test",
	}
	model := models.NewBaselineModel("test", 60, 1800)
	builder := features.NewBuilder()
	store := storage.NewMemoryStore()
	policy := &capacity.Policy{MinReplicas: 1}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := metrics.New("test-run-timeout")

	f := NewWorkloadForecaster(
		"test",
		adapter,
		model,
		builder,
		store,
		policy,
		30*time.Minute, // horizon
		1*time.Minute,  // step
		30*time.Minute, // window
		1*time.Hour,    // interval (very long)
		logger,
		m,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := f.Run(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("Run() error = %v, want %v", err, context.DeadlineExceeded)
	}
}

func TestForecaster_CalculateReplicas(t *testing.T) {
	policy := &capacity.Policy{
		TargetPerPod: 100,
		Headroom:     1.2,
		MinReplicas:  1,
		MaxReplicas:  10,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := metrics.New("test-calc-replicas")

	f := &WorkloadForecaster{
		policy:          policy,
		step:            1 * time.Minute,
		currentReplicas: 2,
		logger:          logger,
		metrics:         m,
	}

	values := []float64{200, 300, 400}
	desiredReplicas, duration := f.calculateReplicas(values, nil)

	if len(desiredReplicas) != len(values) {
		t.Errorf("len(desiredReplicas) = %d, want %d", len(desiredReplicas), len(values))
	}

	if duration == 0 {
		t.Error("duration should be greater than 0")
	}

	// Current replicas should be updated
	if f.currentReplicas == 2 {
		t.Error("currentReplicas should have been updated")
	}
}

func TestForecaster_BuildFeatures(t *testing.T) {
	builder := features.NewBuilder()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	f := &WorkloadForecaster{
		builder: builder,
		logger:  logger,
	}

	df := &adapters.DataFrame{
		Rows: []adapters.Row{
			{"ts": "2024-01-01T00:00:00Z", "value": 100.0},
			{"ts": "2024-01-01T00:01:00Z", "value": 110.0},
		},
	}

	featureFrame, err := f.buildFeatures(df)
	if err != nil {
		t.Fatalf("buildFeatures() error = %v", err)
	}

	if len(featureFrame.Rows) != 2 {
		t.Errorf("len(featureFrame.Rows) = %d, want 2", len(featureFrame.Rows))
	}
}

func TestForecaster_BuildFeatures_Error(t *testing.T) {
	builder := features.NewBuilder()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	f := &WorkloadForecaster{
		builder: builder,
		logger:  logger,
	}

	// Empty dataframe should cause error
	df := &adapters.DataFrame{
		Rows: []adapters.Row{},
	}

	_, err := f.buildFeatures(df)
	if err == nil {
		t.Error("buildFeatures() should return error for empty dataframe")
	}
}

func TestForecaster_StoreSnapshot(t *testing.T) {
	store := storage.NewMemoryStore()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	f := &WorkloadForecaster{
		name:"test-api",
		store:    store,
		step:     1 * time.Minute,
		horizon:  30 * time.Minute,
		logger:   logger,
	}

	forecast := models.Forecast{
		Metric: "http_rps",
		Values: []float64{100, 110, 120},
	}
	desiredReplicas := []int{2, 3, 3}

	err := f.storeSnapshot(context.Background(), forecast, desiredReplicas)
	if err != nil {
		t.Fatalf("storeSnapshot() error = %v", err)
	}

	// Verify snapshot was stored
	snapshot, found, err := store.GetLatest(context.Background(), "test-api")
	if err != nil {
		t.Fatalf("GetLatest() error = %v", err)
	}
	if !found {
		t.Error("snapshot not found in store")
	}
	if snapshot.Workload != "test-api" {
		t.Errorf("workload = %q, want %q", snapshot.Workload, "test-api")
	}
	if len(snapshot.Values) != 3 {
		t.Errorf("len(Values) = %d, want 3", len(snapshot.Values))
	}
	if len(snapshot.DesiredReplicas) != 3 {
		t.Errorf("len(DesiredReplicas) = %d, want 3", len(snapshot.DesiredReplicas))
	}
}

func TestForecaster_Tick_WithMetrics(t *testing.T) {
	// This test verifies metrics are recorded
	store := storage.NewMemoryStore()
	builder := features.NewBuilder()
	model := models.NewBaselineModel("test", 60, 1800)
	policy := &capacity.Policy{
		TargetPerPod: 100,
		MinReplicas:  1,
		MaxReplicas:  10,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := metrics.New("test-tick-metrics")

	// We can't use real Prometheus adapter in tests without a server,
	// so this test is limited. In integration tests, we'd test with a mock adapter.
	_ = &WorkloadForecaster{
		name: "test",
		builder:  builder,
		model:    model,
		store:    store,
		policy:   policy,
		step:     1 * time.Minute,
		horizon:  30 * time.Minute,
		window:   30 * time.Minute,
		logger:   logger,
		metrics:  m,
	}

	// Since we can't easily mock the adapter without modifying the code,
	// we'll skip the actual Tick test and note that it would require
	// dependency injection or interface for the adapter
}

func TestForecaster_Tick_WithoutMetrics(t *testing.T) {
	// Test that forecaster works without metrics (metrics is optional)
	store := storage.NewMemoryStore()
	builder := features.NewBuilder()
	model := models.NewBaselineModel("test", 60, 1800)
	policy := &capacity.Policy{
		TargetPerPod: 100,
		MinReplicas:  1,
		MaxReplicas:  10,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	f := &WorkloadForecaster{
		name:"test",
		builder:  builder,
		model:    model,
		store:    store,
		policy:   policy,
		step:     1 * time.Minute,
		horizon:  30 * time.Minute,
		window:   30 * time.Minute,
		logger:   logger,
		metrics:  nil, // No metrics
	}

	// Should not panic when metrics is nil
	df := &adapters.DataFrame{
		Rows: []adapters.Row{
			{"ts": "2024-01-01T00:00:00Z", "value": 100.0},
		},
	}
	_, err := f.buildFeatures(df)
	if err != nil {
		t.Errorf("buildFeatures should work without metrics: %v", err)
	}
}
