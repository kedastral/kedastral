package adapters

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPAdapter_BasicGET(t *testing.T) {
	// Fake API returning simple JSON array
	json := `{
        "data": [
            {"timestamp": "2025-01-01T00:00:00Z", "value": 100.5},
            {"timestamp": "2025-01-01T00:01:00Z", "value": 110.2},
            {"timestamp": "2025-01-01T00:02:00Z", "value": 120.8}
        ]
    }`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("expected Accept: application/json header")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, json)
	}))
	defer server.Close()

	adapter := &HTTPAdapter{
		URL:             server.URL,
		Method:          "GET",
		ValuePath:       "data.#.value",
		TimestampPath:   "data.#.timestamp",
		TimestampFormat: "rfc3339",
	}

	df, err := adapter.Collect(context.Background(), 600)
	if err != nil {
		t.Fatalf("Collect error: %v", err)
	}
	if df == nil {
		t.Fatalf("expected non-nil DataFrame")
	}
	if len(df.Rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(df.Rows))
	}

	// Verify values
	expectedValues := []float64{100.5, 110.2, 120.8}
	for i, row := range df.Rows {
		if val, ok := row["value"].(float64); !ok || val != expectedValues[i] {
			t.Errorf("row %d: expected value %f, got %v", i, expectedValues[i], row["value"])
		}
		if _, ok := row["ts"].(string); !ok {
			t.Errorf("row %d: timestamp not a string", i)
		}
	}
}

func TestHTTPAdapter_POST_WithBody(t *testing.T) {
	receivedBody := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		// Read body
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		receivedBody = string(buf[:n])

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"results": [{"ts": 1704067200, "val": 42.0}]}`)
	}))
	defer server.Close()

	adapter := &HTTPAdapter{
		URL:    server.URL,
		Method: "POST",
		Body:   `{"window": "{{.WindowSeconds}}s", "step": {{.Step}}}`,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		ValuePath:       "results.#.val",
		TimestampPath:   "results.#.ts",
		TimestampFormat: "unix",
		StepSeconds:     60,
	}

	df, err := adapter.Collect(context.Background(), 3600)
	if err != nil {
		t.Fatalf("Collect error: %v", err)
	}
	if len(df.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(df.Rows))
	}

	// Verify template was rendered
	if receivedBody != `{"window": "3600s", "step": 60}` {
		t.Errorf("unexpected body: %s", receivedBody)
	}

	// Verify value
	if val, ok := df.Rows[0]["value"].(float64); !ok || val != 42.0 {
		t.Errorf("expected value 42.0, got %v", df.Rows[0]["value"])
	}
}

func TestHTTPAdapter_CustomHeaders(t *testing.T) {
	receivedAuth := ""
	receivedCustom := ""

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		receivedCustom = r.Header.Get("X-Custom-Header")

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"metrics": [{"time": "2025-01-01T12:00:00Z", "count": 99}]}`)
	}))
	defer server.Close()

	adapter := &HTTPAdapter{
		URL:    server.URL,
		Method: "GET",
		Headers: map[string]string{
			"Authorization":   "Bearer {{.Token}}",
			"X-Custom-Header": "static-value",
		},
		TemplateVars: map[string]string{
			"Token": "secret123",
		},
		ValuePath:       "metrics.#.count",
		TimestampPath:   "metrics.#.time",
		TimestampFormat: "rfc3339",
	}

	_, err := adapter.Collect(context.Background(), 600)
	if err != nil {
		t.Fatalf("Collect error: %v", err)
	}

	if receivedAuth != "Bearer secret123" {
		t.Errorf("expected 'Bearer secret123', got '%s'", receivedAuth)
	}
	if receivedCustom != "static-value" {
		t.Errorf("expected 'static-value', got '%s'", receivedCustom)
	}
}

func TestHTTPAdapter_UnixTimestamps(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Unix seconds
		fmt.Fprint(w, `{"points": [
			{"ts": 1704067200, "v": 10},
			{"ts": 1704067260, "v": 20}
		]}`)
	}))
	defer server.Close()

	adapter := &HTTPAdapter{
		URL:             server.URL,
		ValuePath:       "points.#.v",
		TimestampPath:   "points.#.ts",
		TimestampFormat: "unix",
	}

	df, err := adapter.Collect(context.Background(), 600)
	if err != nil {
		t.Fatalf("Collect error: %v", err)
	}
	if len(df.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(df.Rows))
	}

	// Verify timestamp parsing
	ts1, _ := time.Parse(time.RFC3339, df.Rows[0]["ts"].(string))
	expected := time.Unix(1704067200, 0).UTC()
	if !ts1.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, ts1)
	}
}

func TestHTTPAdapter_UnixMilliTimestamps(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Unix milliseconds
		fmt.Fprint(w, `{"series": [{"time": 1704067200000, "metric": 5.5}]}`)
	}))
	defer server.Close()

	adapter := &HTTPAdapter{
		URL:             server.URL,
		ValuePath:       "series.#.metric",
		TimestampPath:   "series.#.time",
		TimestampFormat: "unix_milli",
	}

	df, err := adapter.Collect(context.Background(), 600)
	if err != nil {
		t.Fatalf("Collect error: %v", err)
	}
	if len(df.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(df.Rows))
	}

	ts1, _ := time.Parse(time.RFC3339, df.Rows[0]["ts"].(string))
	expected := time.UnixMilli(1704067200000).UTC()
	if !ts1.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, ts1)
	}
}

func TestHTTPAdapter_Sorting(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return data out of order
		fmt.Fprint(w, `{"data": [
			{"ts": "2025-01-01T00:02:00Z", "val": 3},
			{"ts": "2025-01-01T00:00:00Z", "val": 1},
			{"ts": "2025-01-01T00:01:00Z", "val": 2}
		]}`)
	}))
	defer server.Close()

	adapter := &HTTPAdapter{
		URL:             server.URL,
		ValuePath:       "data.#.val",
		TimestampPath:   "data.#.ts",
		TimestampFormat: "rfc3339",
	}

	df, err := adapter.Collect(context.Background(), 600)
	if err != nil {
		t.Fatalf("Collect error: %v", err)
	}

	// Verify sorted order
	prev := time.Time{}
	for i, row := range df.Rows {
		tsStr := row["ts"].(string)
		ts, _ := time.Parse(time.RFC3339, tsStr)
		if !prev.IsZero() && ts.Before(prev) {
			t.Errorf("row %d not sorted: %v before %v", i, ts, prev)
		}
		prev = ts
	}

	// Values should be in time order: 1, 2, 3
	if df.Rows[0]["value"].(float64) != 1 {
		t.Errorf("row 0 should have value 1, got %v", df.Rows[0]["value"])
	}
	if df.Rows[1]["value"].(float64) != 2 {
		t.Errorf("row 1 should have value 2, got %v", df.Rows[1]["value"])
	}
	if df.Rows[2]["value"].(float64) != 3 {
		t.Errorf("row 2 should have value 3, got %v", df.Rows[2]["value"])
	}
}

func TestHTTPAdapter_MismatchedArrayLengths(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// 3 values but only 2 timestamps
		fmt.Fprint(w, `{
			"values": [1, 2, 3],
			"times": ["2025-01-01T00:00:00Z", "2025-01-01T00:01:00Z"]
		}`)
	}))
	defer server.Close()

	adapter := &HTTPAdapter{
		URL:             server.URL,
		ValuePath:       "values",
		TimestampPath:   "times",
		TimestampFormat: "rfc3339",
	}

	_, err := adapter.Collect(context.Background(), 600)
	if err == nil {
		t.Fatal("expected error for mismatched array lengths")
	}
	if !contains(err.Error(), "value count") && !contains(err.Error(), "timestamp count") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestHTTPAdapter_InvalidJSONPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data": [{"val": 1}]}`)
	}))
	defer server.Close()

	adapter := &HTTPAdapter{
		URL:             server.URL,
		ValuePath:       "nonexistent.path",
		TimestampPath:   "data.#.ts",
		TimestampFormat: "rfc3339",
	}

	_, err := adapter.Collect(context.Background(), 600)
	if err == nil {
		t.Fatal("expected error for invalid JSON path")
	}
	if !contains(err.Error(), "not found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestHTTPAdapter_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "internal error")
	}))
	defer server.Close()

	adapter := &HTTPAdapter{
		URL:           server.URL,
		ValuePath:     "data.#.val",
		TimestampPath: "data.#.ts",
	}

	_, err := adapter.Collect(context.Background(), 600)
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
	if !contains(err.Error(), "500") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestHTTPAdapter_ValidatesConfig(t *testing.T) {
	tests := []struct {
		name    string
		adapter *HTTPAdapter
		wantErr bool
	}{
		{
			name:    "missing URL",
			adapter: &HTTPAdapter{ValuePath: "val", TimestampPath: "ts"},
			wantErr: true,
		},
		{
			name:    "missing ValuePath",
			adapter: &HTTPAdapter{URL: "http://example.com", TimestampPath: "ts"},
			wantErr: true,
		},
		{
			name:    "missing TimestampPath",
			adapter: &HTTPAdapter{URL: "http://example.com", ValuePath: "val"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.adapter.Collect(context.Background(), 600)
			if (err != nil) != tt.wantErr {
				t.Errorf("wantErr=%v, got err=%v", tt.wantErr, err)
			}
		})
	}
}

func TestHTTPAdapter_ValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		adapter *HTTPAdapter
		wantErr bool
	}{
		{
			name:    "valid config",
			adapter: &HTTPAdapter{URL: "http://example.com", ValuePath: "v", TimestampPath: "t"},
			wantErr: false,
		},
		{
			name:    "missing URL",
			adapter: &HTTPAdapter{ValuePath: "v", TimestampPath: "t"},
			wantErr: true,
		},
		{
			name:    "invalid timestamp format",
			adapter: &HTTPAdapter{URL: "http://example.com", ValuePath: "v", TimestampPath: "t", TimestampFormat: "invalid"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.adapter.ValidateConfig()
			if (err != nil) != tt.wantErr {
				t.Errorf("wantErr=%v, got err=%v", tt.wantErr, err)
			}
		})
	}
}

func TestHTTPAdapter_Name(t *testing.T) {
	adapter := &HTTPAdapter{}
	if adapter.Name() != "http" {
		t.Errorf("expected 'http', got '%s'", adapter.Name())
	}
}

func TestParseHTTPAdapterConfig(t *testing.T) {
	config := map[string]any{
		"url":             "https://api.example.com",
		"method":          "POST",
		"body":            `{"query": "test"}`,
		"valuePath":       "data.#.value",
		"timestampPath":   "data.#.ts",
		"timestampFormat": "unix",
		"stepSeconds":     120,
		"headers": map[string]any{
			"Authorization": "Bearer token",
		},
		"templateVars": map[string]any{
			"APIKey": "key123",
		},
	}

	adapter, err := ParseHTTPAdapterConfig(config)
	if err != nil {
		t.Fatalf("ParseHTTPAdapterConfig error: %v", err)
	}

	if adapter.URL != "https://api.example.com" {
		t.Errorf("URL: expected 'https://api.example.com', got '%s'", adapter.URL)
	}
	if adapter.Method != "POST" {
		t.Errorf("Method: expected 'POST', got '%s'", adapter.Method)
	}
	if adapter.ValuePath != "data.#.value" {
		t.Errorf("ValuePath: expected 'data.#.value', got '%s'", adapter.ValuePath)
	}
	if adapter.StepSeconds != 120 {
		t.Errorf("StepSeconds: expected 120, got %d", adapter.StepSeconds)
	}
	if adapter.Headers["Authorization"] != "Bearer token" {
		t.Errorf("Headers: expected 'Bearer token', got '%s'", adapter.Headers["Authorization"])
	}
	if adapter.TemplateVars["APIKey"] != "key123" {
		t.Errorf("TemplateVars: expected 'key123', got '%s'", adapter.TemplateVars["APIKey"])
	}
}

func TestParseHTTPAdapterConfig_InvalidStepSeconds(t *testing.T) {
	config := map[string]any{
		"url":           "http://example.com",
		"valuePath":     "v",
		"timestampPath": "t",
		"stepSeconds":   "invalid",
	}

	_, err := ParseHTTPAdapterConfig(config)
	if err == nil {
		t.Fatal("expected error for invalid stepSeconds")
	}
}

func TestMustParseHTTPAdapterConfig(t *testing.T) {
	config := map[string]any{
		"url":           "http://example.com",
		"valuePath":     "v",
		"timestampPath": "t",
	}

	// Should not panic
	adapter := MustParseHTTPAdapterConfig(config)
	if adapter.URL != "http://example.com" {
		t.Errorf("expected 'http://example.com', got '%s'", adapter.URL)
	}
}

func TestMustParseHTTPAdapterConfig_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid config")
		}
	}()

	config := map[string]any{
		"stepSeconds": "invalid",
	}

	MustParseHTTPAdapterConfig(config)
}

func TestHTTPAdapter_ContextCancellation(t *testing.T) {
	// Server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		fmt.Fprint(w, `{"data": []}`)
	}))
	defer server.Close()

	adapter := &HTTPAdapter{
		URL:           server.URL,
		ValuePath:     "data.#.val",
		TimestampPath: "data.#.ts",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := adapter.Collect(ctx, 600)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestHTTPAdapter_DefaultMethod(t *testing.T) {
	receivedMethod := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data": [{"ts": "2025-01-01T00:00:00Z", "val": 1}]}`)
	}))
	defer server.Close()

	adapter := &HTTPAdapter{
		URL:             server.URL,
		// Method not set - should default to GET
		ValuePath:       "data.#.val",
		TimestampPath:   "data.#.ts",
		TimestampFormat: "rfc3339",
	}

	_, err := adapter.Collect(context.Background(), 600)
	if err != nil {
		t.Fatalf("Collect error: %v", err)
	}

	if receivedMethod != http.MethodGet {
		t.Errorf("expected GET, got %s", receivedMethod)
	}
}

func TestHTTPAdapter_TemplateVariables(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data": [{"ts": 1704067200, "val": 1}]}`)
	}))
	defer server.Close()

	adapter := &HTTPAdapter{
		URL:    server.URL,
		Method: "POST",
		Body:   `{"start": {{.Start}}, "end": {{.End}}, "window": {{.WindowSeconds}}, "step": {{.Step}}}`,
		Headers: map[string]string{
			"X-Start": "{{.StartRFC3339}}",
			"X-End":   "{{.EndRFC3339}}",
		},
		ValuePath:       "data.#.val",
		TimestampPath:   "data.#.ts",
		TimestampFormat: "unix",
	}

	// Just verify it doesn't error - we already tested template rendering in other tests
	_, err := adapter.Collect(context.Background(), 3600)
	if err != nil {
		t.Fatalf("Collect error: %v", err)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
