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

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/HatiCode/kedastral/pkg/client"
	"github.com/HatiCode/kedastral/pkg/storage"
)

func makeSnapshotServer(t *testing.T, workload string, snap storage.Snapshot, stale bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/workloads" {
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{"workloads": []string{workload}}); err != nil {
				t.Errorf("encode workloads: %v", err)
			}
			return
		}
		if r.URL.Path == "/forecast/current" {
			if stale {
				w.Header().Set("X-Kedastral-Stale", "true")
			}
			w.Header().Set("Content-Type", "application/json")
			resp := client.SnapshotResponse{
				Workload:        snap.Workload,
				Metric:          snap.Metric,
				GeneratedAt:     snap.GeneratedAt,
				StepSeconds:     snap.StepSeconds,
				HorizonSeconds:  snap.HorizonSeconds,
				Values:          snap.Values,
				DesiredReplicas: snap.DesiredReplicas,
			}
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Errorf("encode snapshot: %v", err)
			}
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
}

func callToolRequest(args map[string]any) mcp.CallToolRequest {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	return req
}

// --- list_workloads ---

func TestHandleListWorkloads_Success(t *testing.T) {
	srv := makeSnapshotServer(t, "my-api", storage.Snapshot{}, false)
	defer srv.Close()

	fc := client.NewForecasterClient(srv.URL)
	handler := handleListWorkloads(fc, discardLogger())

	result, err := handler(context.Background(), callToolRequest(nil))
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}

	text := extractText(t, result)
	if !containsStr(text, "my-api") {
		t.Errorf("expected my-api in response, got %q", text)
	}
}

func TestHandleListWorkloads_ForecasterDown(t *testing.T) {
	fc := client.NewForecasterClient("http://127.0.0.1:1") // nothing listening
	handler := handleListWorkloads(fc, discardLogger())

	result, err := handler(context.Background(), callToolRequest(nil))
	if err != nil {
		t.Fatalf("handler should not return error, got %v", err)
	}
	if !result.IsError {
		t.Fatal("expected tool error result when forecaster is unreachable")
	}
}

// --- get_forecast ---

func TestHandleGetForecast_Success(t *testing.T) {
	snap := storage.Snapshot{
		Workload:        "my-api",
		Metric:          "http_rps",
		GeneratedAt:     time.Now(),
		StepSeconds:     60,
		HorizonSeconds:  1800,
		Values:          []float64{100, 110, 120},
		DesiredReplicas: []int{2, 3, 3},
	}
	srv := makeSnapshotServer(t, "my-api", snap, false)
	defer srv.Close()

	fc := client.NewForecasterClient(srv.URL)
	handler := handleGetForecast(fc, 5*time.Minute, discardLogger())

	result, err := handler(context.Background(), callToolRequest(map[string]any{"workload": "my-api"}))
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}

	text := extractText(t, result)
	for _, want := range []string{"my-api", "http_rps", "100.00", "2, 3, 3"} {
		if !containsStr(text, want) {
			t.Errorf("expected %q in response, got %q", want, text)
		}
	}
}

func TestHandleGetForecast_MissingWorkload(t *testing.T) {
	fc := client.NewForecasterClient("http://127.0.0.1:1")
	handler := handleGetForecast(fc, 5*time.Minute, discardLogger())

	result, err := handler(context.Background(), callToolRequest(nil))
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if !result.IsError {
		t.Fatal("expected tool error for missing workload argument")
	}
}

func TestHandleGetForecast_StaleSnapshot(t *testing.T) {
	snap := storage.Snapshot{
		Workload:        "my-api",
		GeneratedAt:     time.Now().Add(-10 * time.Minute),
		Values:          []float64{50},
		DesiredReplicas: []int{2},
	}
	srv := makeSnapshotServer(t, "my-api", snap, true)
	defer srv.Close()

	fc := client.NewForecasterClient(srv.URL)
	handler := handleGetForecast(fc, 5*time.Minute, discardLogger())

	result, err := handler(context.Background(), callToolRequest(map[string]any{"workload": "my-api"}))
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}

	text := extractText(t, result)
	if !containsStr(text, "STALE") {
		t.Errorf("expected STALE marker in response, got %q", text)
	}
}

// --- explain_decision ---

func TestHandleExplainDecision_ScalingUp(t *testing.T) {
	snap := storage.Snapshot{
		Workload:        "my-api",
		Metric:          "http_rps",
		GeneratedAt:     time.Now(),
		StepSeconds:     60,
		HorizonSeconds:  1800,
		Values:          []float64{100, 150, 200},
		DesiredReplicas: []int{2, 4, 6},
	}
	srv := makeSnapshotServer(t, "my-api", snap, false)
	defer srv.Close()

	fc := client.NewForecasterClient(srv.URL)
	handler := handleExplainDecision(fc, 5*time.Minute, discardLogger())

	result, err := handler(context.Background(), callToolRequest(map[string]any{"workload": "my-api"}))
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}

	text := extractText(t, result)
	if !containsStr(text, "scaling up") {
		t.Errorf("expected 'scaling up' in trend, got %q", text)
	}
}

func TestHandleExplainDecision_MissingWorkload(t *testing.T) {
	fc := client.NewForecasterClient("http://127.0.0.1:1")
	handler := handleExplainDecision(fc, 5*time.Minute, discardLogger())

	result, err := handler(context.Background(), callToolRequest(nil))
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if !result.IsError {
		t.Fatal("expected tool error for missing workload argument")
	}
}

// --- pure function tests ---

func TestAnalyzeTrend(t *testing.T) {
	tests := []struct {
		name     string
		replicas []int
		wantSub  string
	}{
		{"empty", nil, "insufficient data"},
		{"single", []int{5}, "insufficient data"},
		{"stable", []int{3, 3, 3}, "stable"},
		{"scaling up", []int{2, 3, 4, 5, 6}, "scaling up"},
		{"scaling down", []int{6, 5, 4, 3, 2}, "scaling down"},
		{"spike then down", []int{2, 8, 2}, "spike then scale down"},
		{"dip then up", []int{8, 2, 8}, "dip then scale up"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := analyzeTrend(tt.replicas)
			if !containsStr(got, tt.wantSub) {
				t.Errorf("analyzeTrend(%v) = %q, want substring %q", tt.replicas, got, tt.wantSub)
			}
		})
	}
}

func TestMaxReplicas(t *testing.T) {
	max, idx := maxReplicas([]int{2, 5, 3, 1})
	if max != 5 || idx != 1 {
		t.Errorf("maxReplicas = (%d, %d), want (5, 1)", max, idx)
	}
}

func TestMinReplicasVal(t *testing.T) {
	min, idx := minReplicasVal([]int{5, 2, 8, 1})
	if min != 1 || idx != 3 {
		t.Errorf("minReplicasVal = (%d, %d), want (1, 3)", min, idx)
	}
}

func TestMinReplicasVal_Empty(t *testing.T) {
	min, idx := minReplicasVal(nil)
	if min != 0 || idx != 0 {
		t.Errorf("minReplicasVal(nil) = (%d, %d), want (0, 0)", min, idx)
	}
}

func TestFormatFloats(t *testing.T) {
	got := formatFloats([]float64{1.5, 2.0, 3.333})
	want := "1.50, 2.00, 3.33"
	if got != want {
		t.Errorf("formatFloats = %q, want %q", got, want)
	}
}

func TestFormatFloats_Empty(t *testing.T) {
	if got := formatFloats(nil); got != "(none)" {
		t.Errorf("formatFloats(nil) = %q, want (none)", got)
	}
}

func TestFormatInts(t *testing.T) {
	got := formatInts([]int{1, 2, 3})
	want := "1, 2, 3"
	if got != want {
		t.Errorf("formatInts = %q, want %q", got, want)
	}
}

func TestFormatInts_Empty(t *testing.T) {
	if got := formatInts(nil); got != "(none)" {
		t.Errorf("formatInts(nil) = %q, want (none)", got)
	}
}

// --- helpers ---

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func extractText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("result has no content")
	}
	text, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	return text.Text
}

func containsStr(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && findStr(s, sub))
}

func findStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
