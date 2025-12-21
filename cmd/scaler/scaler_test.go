package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HatiCode/kedastral/cmd/scaler/metrics"
	pb "github.com/HatiCode/kedastral/pkg/api/externalscaler"
	"github.com/HatiCode/kedastral/pkg/storage"
	"github.com/HatiCode/kedastral/pkg/tls"
)

// Shared metrics instance for all tests to avoid duplicate registration
var scalerTestMetrics = metrics.New()

func TestNew(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := scalerTestMetrics

	s, err := New("http://localhost:8081", 5*time.Minute, tls.Config{Enabled: false}, logger, m)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if s == nil {
		t.Fatal("New() returned nil")
	}
	if s.forecasterURL != "http://localhost:8081" {
		t.Errorf("forecasterURL = %q, want %q", s.forecasterURL, "http://localhost:8081")
	}
	if s.leadTime != 5*time.Minute {
		t.Errorf("leadTime = %v, want 5m", s.leadTime)
	}
}

func TestNew_NilLogger(t *testing.T) {
	m := scalerTestMetrics

	s, err := New("http://localhost:8081", 5*time.Minute, tls.Config{Enabled: false}, nil, m)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if s.logger == nil {
		t.Error("logger should not be nil when nil is passed")
	}
}

func TestScaler_GetWorkload(t *testing.T) {
	s := &Scaler{}

	tests := []struct {
		name string
		ref  *pb.ScaledObjectRef
		want string
	}{
		{
			name: "workload from metadata",
			ref: &pb.ScaledObjectRef{
				Name: "my-deployment",
				ScalerMetadata: map[string]string{
					"workload": "my-api",
				},
			},
			want: "my-api",
		},
		{
			name: "workload from name",
			ref: &pb.ScaledObjectRef{
				Name:           "my-deployment",
				ScalerMetadata: map[string]string{},
			},
			want: "my-deployment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.getWorkload(tt.ref)
			if got != tt.want {
				t.Errorf("getWorkload() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestScaler_GetMetricName(t *testing.T) {
	s := &Scaler{}

	tests := []struct {
		name     string
		ref      *pb.ScaledObjectRef
		workload string
		want     string
	}{
		{
			name: "custom metric name",
			ref: &pb.ScaledObjectRef{
				ScalerMetadata: map[string]string{
					"metricName": "custom-metric",
				},
			},
			workload: "my-api",
			want:     "custom-metric",
		},
		{
			name: "default metric name",
			ref: &pb.ScaledObjectRef{
				ScalerMetadata: map[string]string{},
			},
			workload: "my-api",
			want:     "kedastral-my-api-desired-replicas",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.getMetricName(tt.ref, tt.workload)
			if got != tt.want {
				t.Errorf("getMetricName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestScaler_SelectReplicasAtLeadTime(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tests := []struct {
		name     string
		leadTime time.Duration
		snapshot *storage.Snapshot
		want     int
	}{
		{
			name:     "lead time of 5 minutes",
			leadTime: 5 * time.Minute,
			snapshot: &storage.Snapshot{
				StepSeconds:     60,
				DesiredReplicas: []int{2, 2, 3, 3, 4, 5, 5, 5},
			},
			want: 5, // index 5 (5 minutes / 1 minute steps)
		},
		{
			name:     "lead time of 0",
			leadTime: 0,
			snapshot: &storage.Snapshot{
				StepSeconds:     60,
				DesiredReplicas: []int{2, 3, 4, 5},
			},
			want: 2, // index 0
		},
		{
			name:     "lead time exceeds data",
			leadTime: 30 * time.Minute,
			snapshot: &storage.Snapshot{
				StepSeconds:     60,
				DesiredReplicas: []int{2, 3, 4},
			},
			want: 4, // last index (2)
		},
		{
			name:     "empty replicas",
			leadTime: 5 * time.Minute,
			snapshot: &storage.Snapshot{
				StepSeconds:     60,
				DesiredReplicas: []int{},
			},
			want: 1, // default
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Scaler{
				leadTime: tt.leadTime,
				logger:   logger,
			}

			got := s.selectReplicasAtLeadTime(tt.snapshot)
			if got != tt.want {
				t.Errorf("selectReplicasAtLeadTime() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestScaler_IsActive_Success(t *testing.T) {
	// Create a test HTTP server that returns a valid forecast
	snapshot := storage.Snapshot{
		Workload:        "test-api",
		Metric:          "http_rps",
		GeneratedAt:     time.Now(),
		StepSeconds:     60,
		HorizonSeconds:  1800,
		Values:          []float64{100, 110, 120},
		DesiredReplicas: []int{2, 3, 3},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(snapshot); err != nil {
			t.Errorf("failed to encode snapshot: %v", err)
		}
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := scalerTestMetrics
	s, err := New(server.URL, 5*time.Minute, tls.Config{Enabled: false}, logger, m)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ref := &pb.ScaledObjectRef{
		Name:           "test-api",
		Namespace:      "default",
		ScalerMetadata: map[string]string{},
	}

	resp, err := s.IsActive(context.Background(), ref)
	if err != nil {
		t.Fatalf("IsActive() error = %v", err)
	}

	if !resp.Result {
		t.Error("IsActive() should return true for fresh forecast")
	}
}

func TestScaler_IsActive_Stale(t *testing.T) {
	// Create a test HTTP server that returns a stale forecast
	snapshot := storage.Snapshot{
		Workload:        "test-api",
		Metric:          "http_rps",
		GeneratedAt:     time.Now().Add(-20 * time.Minute), // Very old
		StepSeconds:     60,
		HorizonSeconds:  1800,
		Values:          []float64{100, 110, 120},
		DesiredReplicas: []int{2, 3, 3},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(snapshot); err != nil {
			t.Errorf("failed to encode snapshot: %v", err)
		}
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := scalerTestMetrics
	s, err := New(server.URL, 5*time.Minute, tls.Config{Enabled: false}, logger, m)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ref := &pb.ScaledObjectRef{
		Name:           "test-api",
		Namespace:      "default",
		ScalerMetadata: map[string]string{},
	}

	resp, err := s.IsActive(context.Background(), ref)
	if err != nil {
		t.Fatalf("IsActive() error = %v", err)
	}

	if resp.Result {
		t.Error("IsActive() should return false for stale forecast")
	}
}

func TestScaler_IsActive_Error(t *testing.T) {
	// Create a test HTTP server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := scalerTestMetrics
	s, err := New(server.URL, 5*time.Minute, tls.Config{Enabled: false}, logger, m)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ref := &pb.ScaledObjectRef{
		Name:           "test-api",
		Namespace:      "default",
		ScalerMetadata: map[string]string{},
	}

	resp, err := s.IsActive(context.Background(), ref)
	if err != nil {
		t.Fatalf("IsActive() error = %v", err)
	}

	if resp.Result {
		t.Error("IsActive() should return false on error")
	}
}

func TestScaler_GetMetricSpec(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := scalerTestMetrics
	s, err := New("http://localhost:8081", 5*time.Minute, tls.Config{Enabled: false}, logger, m)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ref := &pb.ScaledObjectRef{
		Name:      "test-api",
		Namespace: "default",
		ScalerMetadata: map[string]string{
			"workload": "my-api",
		},
	}

	resp, err := s.GetMetricSpec(context.Background(), ref)
	if err != nil {
		t.Fatalf("GetMetricSpec() error = %v", err)
	}

	if len(resp.MetricSpecs) != 1 {
		t.Fatalf("len(MetricSpecs) = %d, want 1", len(resp.MetricSpecs))
	}

	spec := resp.MetricSpecs[0]
	if spec.MetricName != "kedastral-my-api-desired-replicas" {
		t.Errorf("MetricName = %q, want %q", spec.MetricName, "kedastral-my-api-desired-replicas")
	}
	if spec.TargetSizeFloat != 1.0 {
		t.Errorf("TargetSizeFloat = %f, want 1.0", spec.TargetSizeFloat)
	}
}

func TestScaler_GetMetrics_Success(t *testing.T) {
	// Create a test HTTP server that returns a valid forecast
	snapshot := storage.Snapshot{
		Workload:        "test-api",
		Metric:          "http_rps",
		GeneratedAt:     time.Now(),
		StepSeconds:     60,
		HorizonSeconds:  1800,
		Values:          []float64{100, 110, 120, 130, 140, 150},
		DesiredReplicas: []int{2, 2, 3, 3, 4, 5},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(snapshot); err != nil {
			t.Errorf("failed to encode snapshot: %v", err)
		}
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := scalerTestMetrics
	s, err := New(server.URL, 5*time.Minute, tls.Config{Enabled: false}, logger, m)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	req := &pb.GetMetricsRequest{
		ScaledObjectRef: &pb.ScaledObjectRef{
			Name:           "test-api",
			Namespace:      "default",
			ScalerMetadata: map[string]string{},
		},
		MetricName: "kedastral-test-api-desired-replicas",
	}

	resp, err := s.GetMetrics(context.Background(), req)
	if err != nil {
		t.Fatalf("GetMetrics() error = %v", err)
	}

	if len(resp.MetricValues) != 1 {
		t.Fatalf("len(MetricValues) = %d, want 1", len(resp.MetricValues))
	}

	value := resp.MetricValues[0]
	if value.MetricName != "kedastral-test-api-desired-replicas" {
		t.Errorf("MetricName = %q, want %q", value.MetricName, "kedastral-test-api-desired-replicas")
	}

	// Lead time of 5 minutes with 1 minute steps = index 5
	// DesiredReplicas[5] = 5
	if value.MetricValueFloat != 5.0 {
		t.Errorf("MetricValueFloat = %f, want 5.0", value.MetricValueFloat)
	}
}

func TestScaler_GetMetrics_Error(t *testing.T) {
	// Create a test HTTP server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := scalerTestMetrics
	s, err := New(server.URL, 5*time.Minute, tls.Config{Enabled: false}, logger, m)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	req := &pb.GetMetricsRequest{
		ScaledObjectRef: &pb.ScaledObjectRef{
			Name:           "test-api",
			Namespace:      "default",
			ScalerMetadata: map[string]string{},
		},
		MetricName: "kedastral-test-api-desired-replicas",
	}

	_, err = s.GetMetrics(context.Background(), req)
	if err == nil {
		t.Error("GetMetrics() should return error when forecast fetch fails")
	}
}

func TestScaler_StreamIsActive_NotImplemented(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := scalerTestMetrics
	s, err := New("http://localhost:8081", 5*time.Minute, tls.Config{Enabled: false}, logger, m)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ref := &pb.ScaledObjectRef{
		Name:           "test-api",
		Namespace:      "default",
		ScalerMetadata: map[string]string{},
	}

	err = s.StreamIsActive(ref, nil)
	if err == nil {
		t.Error("StreamIsActive() should return error (not implemented)")
	}
}
