package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HatiCode/kedastral/pkg/storage"
)

func TestNewForecasterClient(t *testing.T) {
	client := NewForecasterClient("http://localhost:8081")
	if client == nil {
		t.Fatal("NewForecasterClient returned nil")
	}
	if client.baseURL != "http://localhost:8081" {
		t.Errorf("baseURL = %q, want %q", client.baseURL, "http://localhost:8081")
	}
	if client.httpClient.Timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", client.httpClient.Timeout)
	}
}

func TestNewForecasterClientWithTimeout(t *testing.T) {
	timeout := 10 * time.Second
	client := NewForecasterClientWithTimeout("http://localhost:8081", timeout)
	if client.httpClient.Timeout != timeout {
		t.Errorf("timeout = %v, want %v", client.httpClient.Timeout, timeout)
	}
}

func TestForecasterClient_GetSnapshot_Success(t *testing.T) {
	// Create fake forecaster server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/forecast/current" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("workload") != "test-api" {
			t.Errorf("unexpected workload: %s", r.URL.Query().Get("workload"))
		}

		resp := SnapshotResponse{
			Workload:        "test-api",
			Metric:          "http_rps",
			GeneratedAt:     time.Now(),
			StepSeconds:     60,
			HorizonSeconds:  1800,
			Values:          []float64{100, 110, 120},
			DesiredReplicas: []int{2, 3, 3},
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client := NewForecasterClient(server.URL)
	result, err := client.GetSnapshot(context.Background(), "test-api")
	if err != nil {
		t.Fatalf("GetSnapshot() error = %v", err)
	}

	if result.Stale {
		t.Error("Expected Stale = false")
	}

	snapshot := result.Snapshot
	if snapshot.Workload != "test-api" {
		t.Errorf("Workload = %q, want %q", snapshot.Workload, "test-api")
	}
	if snapshot.Metric != "http_rps" {
		t.Errorf("Metric = %q, want %q", snapshot.Metric, "http_rps")
	}
	if len(snapshot.Values) != 3 {
		t.Errorf("len(Values) = %d, want 3", len(snapshot.Values))
	}
	if len(snapshot.DesiredReplicas) != 3 {
		t.Errorf("len(DesiredReplicas) = %d, want 3", len(snapshot.DesiredReplicas))
	}
}

func TestForecasterClient_GetSnapshot_Stale(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set stale header per SPEC.md §3.1
		w.Header().Set("X-Kedastral-Stale", "true")
		w.Header().Set("Content-Type", "application/json")

		resp := SnapshotResponse{
			Workload:    "test-api",
			GeneratedAt: time.Now().Add(-5 * time.Minute),
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client := NewForecasterClient(server.URL)
	result, err := client.GetSnapshot(context.Background(), "test-api")
	if err != nil {
		t.Fatalf("GetSnapshot() error = %v", err)
	}

	if !result.Stale {
		t.Error("Expected Stale = true")
	}
}

func TestForecasterClient_GetSnapshot_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "snapshot not found"}); err != nil {
			t.Errorf("failed to encode error response: %v", err)
		}
	}))
	defer server.Close()

	client := NewForecasterClient(server.URL)
	_, err := client.GetSnapshot(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("Expected error for not found snapshot")
	}
}

func TestForecasterClient_GetSnapshot_EmptyWorkload(t *testing.T) {
	client := NewForecasterClient("http://localhost:8081")
	_, err := client.GetSnapshot(context.Background(), "")
	if err == nil {
		t.Fatal("Expected error for empty workload")
	}
}

func TestForecasterClient_GetSnapshot_InvalidURL(t *testing.T) {
	client := NewForecasterClient("://invalid-url")
	_, err := client.GetSnapshot(context.Background(), "test")
	if err == nil {
		t.Fatal("Expected error for invalid URL")
	}
}

func TestForecasterClient_GetSnapshot_ContextCancellation(t *testing.T) {
	// Server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		if err := json.NewEncoder(w).Encode(SnapshotResponse{}); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client := NewForecasterClient(server.URL)

	// Cancel context immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.GetSnapshot(ctx, "test-api")
	if err == nil {
		t.Fatal("Expected error for cancelled context")
	}
}

func TestForecasterClient_GetSnapshot_Timeout(t *testing.T) {
	// Server that delays response longer than client timeout
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		if err := json.NewEncoder(w).Encode(SnapshotResponse{}); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	// Client with very short timeout
	client := NewForecasterClientWithTimeout(server.URL, 10*time.Millisecond)

	_, err := client.GetSnapshot(context.Background(), "test-api")
	if err == nil {
		t.Fatal("Expected timeout error")
	}
}

func TestForecasterClient_GetSnapshot_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte("invalid json")); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	client := NewForecasterClient(server.URL)
	_, err := client.GetSnapshot(context.Background(), "test-api")
	if err == nil {
		t.Fatal("Expected error for invalid JSON")
	}
}

func TestForecasterClient_GetSnapshot_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewForecasterClient(server.URL)
	_, err := client.GetSnapshot(context.Background(), "test-api")
	if err == nil {
		t.Fatal("Expected error for server error")
	}
}

func TestIsStale(t *testing.T) {
	tests := []struct {
		name        string
		generatedAt time.Time
		staleAfter  time.Duration
		want        bool
	}{
		{
			name:        "fresh snapshot",
			generatedAt: time.Now().Add(-30 * time.Second),
			staleAfter:  2 * time.Minute,
			want:        false,
		},
		{
			name:        "stale snapshot",
			generatedAt: time.Now().Add(-5 * time.Minute),
			staleAfter:  2 * time.Minute,
			want:        true,
		},
		{
			name:        "just before threshold",
			generatedAt: time.Now().Add(-1*time.Minute - 59*time.Second),
			staleAfter:  2 * time.Minute,
			want:        false, // Should be fresh
		},
		{
			name:        "very old snapshot",
			generatedAt: time.Now().Add(-1 * time.Hour),
			staleAfter:  2 * time.Minute,
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := storage.Snapshot{
				GeneratedAt: tt.generatedAt,
			}
			got := IsStale(snapshot, tt.staleAfter)
			if got != tt.want {
				t.Errorf("IsStale() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestForecasterClient_ListWorkloads_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/workloads" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"workloads": []string{"api-1", "api-2"}}); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
	defer srv.Close()

	c := NewForecasterClient(srv.URL)
	workloads, err := c.ListWorkloads(context.Background())
	if err != nil {
		t.Fatalf("ListWorkloads() error = %v", err)
	}
	if len(workloads) != 2 {
		t.Fatalf("len(workloads) = %d, want 2", len(workloads))
	}
	got := map[string]bool{workloads[0]: true, workloads[1]: true}
	for _, w := range []string{"api-1", "api-2"} {
		if !got[w] {
			t.Errorf("missing workload %q in response", w)
		}
	}
}

func TestForecasterClient_ListWorkloads_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"workloads": []string{}}); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
	defer srv.Close()

	c := NewForecasterClient(srv.URL)
	workloads, err := c.ListWorkloads(context.Background())
	if err != nil {
		t.Fatalf("ListWorkloads() error = %v", err)
	}
	if len(workloads) != 0 {
		t.Errorf("expected empty slice, got %v", workloads)
	}
}

func TestForecasterClient_ListWorkloads_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewForecasterClient(srv.URL)
	_, err := c.ListWorkloads(context.Background())
	if err == nil {
		t.Fatal("expected error for server error")
	}
}

func TestForecasterClient_ListWorkloads_InvalidURL(t *testing.T) {
	c := NewForecasterClient("://invalid")
	_, err := c.ListWorkloads(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestForecasterClient_ListWorkloads_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	defer srv.Close()

	c := NewForecasterClient(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.ListWorkloads(ctx)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestForecasterClient_GetSnapshot_URLConstruction(t *testing.T) {
	// Verify URL is constructed correctly with special characters
	var capturedURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(SnapshotResponse{Workload: "test"}); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client := NewForecasterClient(server.URL)
	if _, err := client.GetSnapshot(context.Background(), "my-api-prod"); err != nil {
		t.Errorf("GetSnapshot() error = %v", err)
	}

	expectedPath := "/forecast/current?workload=my-api-prod"
	if capturedURL != expectedPath {
		t.Errorf("URL = %q, want %q", capturedURL, expectedPath)
	}
}
