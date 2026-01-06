package adapters

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestVictoriaMetricsAdapter_SingleSeries(t *testing.T) {
	// Fake VictoriaMetrics server returning 3 points (Prometheus-compatible response)
	json := `{
        "status":"success",
        "data":{
            "resultType":"matrix",
            "result":[
                {
                    "metric":{},
                    "values":[
                        [ 1700000000, "100" ],
                        [ 1700000060, "110" ],
                        [ 1700000120, "120" ]
                    ]
                }
            ]
        }
    }`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, json)
	}))
	defer server.Close()

	ad := &VictoriaMetricsAdapter{
		ServerURL:   server.URL,
		Query:       "sum(rate(http_requests_total[1m]))",
		StepSeconds: 60,
	}

	df, err := ad.Collect(context.Background(), 600)
	if err != nil {
		t.Fatalf("Collect error: %v", err)
	}
	if df == nil {
		t.Fatalf("expected non-nil DataFrame pointer")
	}
	if len(df.Rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(df.Rows))
	}

	// Check ordering and types
	prev := time.Time{}
	for i, row := range df.Rows {
		tsStr, ok := row["ts"].(string)
		if !ok {
			t.Fatalf("row %d ts not string", i)
		}
		ts, err := time.Parse(time.RFC3339, tsStr)
		if err != nil {
			t.Fatalf("row %d ts parse: %v", i, err)
		}
		if !prev.IsZero() && ts.Before(prev) {
			t.Fatalf("timestamps not sorted")
		}
		prev = ts
		if _, ok := row["value"].(float64); !ok {
			t.Fatalf("row %d value not float64", i)
		}
	}
}

func TestVictoriaMetricsAdapter_MultiSeriesAggregates(t *testing.T) {
	json := `{
        "status":"success",
        "data":{
            "resultType":"matrix",
            "result":[
                { "metric":{}, "values":[ [ 1700000000, "1" ], [ 1700000060, "2" ] ] },
                { "metric":{}, "values":[ [ 1700000000, "10" ], [ 1700000060, "20" ] ] }
            ]
        }
    }`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, json)
	}))
	defer server.Close()

	ad := &VictoriaMetricsAdapter{ServerURL: server.URL, Query: "q", StepSeconds: 60}
	df, err := ad.Collect(context.Background(), 120)
	if err != nil {
		t.Fatalf("Collect error: %v", err)
	}
	if df == nil {
		t.Fatalf("expected non-nil DataFrame pointer")
	}
	if len(df.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(df.Rows))
	}
	// Values should be summed: (1+10)=11, (2+20)=22
	if df.Rows[0]["value"].(float64) != 11 {
		t.Fatalf("row0 value = %v, want 11", df.Rows[0]["value"])
	}
	if df.Rows[1]["value"].(float64) != 22 {
		t.Fatalf("row1 value = %v, want 22", df.Rows[1]["value"])
	}
}

func TestVictoriaMetricsAdapter_ValidatesConfig(t *testing.T) {
	ad := &VictoriaMetricsAdapter{}
	if _, err := ad.Collect(context.Background(), 60); err == nil {
		t.Fatalf("expected error for missing config")
	}
}

func TestVictoriaMetricsAdapter_Name(t *testing.T) {
	ad := &VictoriaMetricsAdapter{}
	if name := ad.Name(); name != "victoria-metrics" {
		t.Fatalf("expected name 'victoria-metrics', got %q", name)
	}
}

func TestVictoriaMetricsAdapter_ContextCancellation(t *testing.T) {
	// Server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		fmt.Fprint(w, `{"status":"success","data":{"resultType":"matrix","result":[]}}`)
	}))
	defer server.Close()

	ad := &VictoriaMetricsAdapter{
		ServerURL:   server.URL,
		Query:       "test",
		StepSeconds: 60,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := ad.Collect(ctx, 60)
	if err == nil {
		t.Fatalf("expected error due to context cancellation")
	}
}

func TestVictoriaMetricsAdapter_DefaultStep(t *testing.T) {
	json := `{
        "status":"success",
        "data":{
            "resultType":"matrix",
            "result":[]
        }
    }`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify step parameter is set to default 60
		if step := r.URL.Query().Get("step"); step != "60" {
			t.Errorf("expected step=60, got step=%s", step)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, json)
	}))
	defer server.Close()

	ad := &VictoriaMetricsAdapter{
		ServerURL:   server.URL,
		Query:       "test",
		StepSeconds: 0, // Should default to 60
	}

	_, err := ad.Collect(context.Background(), 120)
	if err != nil {
		t.Fatalf("Collect error: %v", err)
	}
}
