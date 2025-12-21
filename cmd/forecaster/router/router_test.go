package router

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HatiCode/kedastral/pkg/storage"
)

func TestSetupRoutes(t *testing.T) {
	store := storage.NewMemoryStore()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	mux := SetupRoutes(store, 2*time.Minute, logger)

	if mux == nil {
		t.Fatal("SetupRoutes() returned nil")
	}
}

func TestHealthEndpoint(t *testing.T) {
	store := storage.NewMemoryStore()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux := SetupRoutes(store, 2*time.Minute, logger)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if body != "OK" {
		t.Errorf("body = %q, want %q", body, "OK")
	}
}

func TestMetricsEndpoint(t *testing.T) {
	store := storage.NewMemoryStore()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux := SetupRoutes(store, 2*time.Minute, logger)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	// Metrics endpoint should return prometheus text format
	contentType := w.Header().Get("Content-Type")
	if contentType == "" {
		t.Error("Content-Type header should be set for metrics endpoint")
	}
}

func TestGetSnapshot_MissingWorkload(t *testing.T) {
	store := storage.NewMemoryStore()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux := SetupRoutes(store, 2*time.Minute, logger)

	req := httptest.NewRequest(http.MethodGet, "/forecast/current", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestGetSnapshot_NotFound(t *testing.T) {
	store := storage.NewMemoryStore()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux := SetupRoutes(store, 2*time.Minute, logger)

	req := httptest.NewRequest(http.MethodGet, "/forecast/current?workload=nonexistent", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestGetSnapshot_Success(t *testing.T) {
	store := storage.NewMemoryStore()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Store a snapshot
	snapshot := storage.Snapshot{
		Workload:        "test-api",
		Metric:          "http_rps",
		GeneratedAt:     time.Now(),
		StepSeconds:     60,
		HorizonSeconds:  1800,
		Values:          []float64{100, 110, 120},
		DesiredReplicas: []int{2, 3, 3},
	}
	err := store.Put(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("failed to put snapshot: %v", err)
	}

	mux := SetupRoutes(store, 2*time.Minute, logger)

	req := httptest.NewRequest(http.MethodGet, "/forecast/current?workload=test-api", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	// Check Content-Type
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}

	// Check that X-Kedastral-Stale is not set (snapshot is fresh)
	staleHeader := w.Header().Get("X-Kedastral-Stale")
	if staleHeader == "true" {
		t.Error("snapshot should not be marked as stale")
	}
}

func TestGetSnapshot_Stale(t *testing.T) {
	store := storage.NewMemoryStore()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Store an old snapshot
	snapshot := storage.Snapshot{
		Workload:        "test-api",
		Metric:          "http_rps",
		GeneratedAt:     time.Now().Add(-5 * time.Minute), // 5 minutes ago
		StepSeconds:     60,
		HorizonSeconds:  1800,
		Values:          []float64{100, 110, 120},
		DesiredReplicas: []int{2, 3, 3},
	}
	err := store.Put(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("failed to put snapshot: %v", err)
	}

	mux := SetupRoutes(store, 2*time.Minute, logger) // Stale after 2 minutes

	req := httptest.NewRequest(http.MethodGet, "/forecast/current?workload=test-api", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	// Check that X-Kedastral-Stale header is set
	staleHeader := w.Header().Get("X-Kedastral-Stale")
	if staleHeader != "true" {
		t.Error("snapshot should be marked as stale")
	}
}

func TestGetSnapshot_JSONResponse(t *testing.T) {
	store := storage.NewMemoryStore()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	now := time.Now()
	snapshot := storage.Snapshot{
		Workload:        "test-api",
		Metric:          "http_rps",
		GeneratedAt:     now,
		StepSeconds:     60,
		HorizonSeconds:  1800,
		Values:          []float64{100, 110, 120},
		DesiredReplicas: []int{2, 3, 3},
	}
	if err := store.Put(context.Background(), snapshot); err != nil {
		t.Fatalf("failed to put snapshot: %v", err)
	}

	mux := SetupRoutes(store, 2*time.Minute, logger)

	req := httptest.NewRequest(http.MethodGet, "/forecast/current?workload=test-api", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	// Parse JSON and verify fields
	body := w.Body.String()
	if body == "" {
		t.Fatal("response body is empty")
	}

	// Check for expected JSON fields
	expectedFields := []string{
		"\"workload\"",
		"\"metric\"",
		"\"generatedAt\"",
		"\"stepSeconds\"",
		"\"horizonSeconds\"",
		"\"values\"",
		"\"desiredReplicas\"",
	}

	for _, field := range expectedFields {
		if !contains(body, field) {
			t.Errorf("response missing field %s", field)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
