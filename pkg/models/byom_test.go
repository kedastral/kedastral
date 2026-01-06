package models

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBYOMModel_Name(t *testing.T) {
	model := NewBYOMModel("http://localhost:8082/predict", "test_metric", 60, 1800)
	if model.Name() != "byom" {
		t.Errorf("expected name 'byom', got %q", model.Name())
	}
}

func TestBYOMModel_Train(t *testing.T) {
	model := NewBYOMModel("http://localhost:8082/predict", "test_metric", 60, 1800)
	features := FeatureFrame{
		Rows: []map[string]float64{
			{"ts": 1609459200, "value": 100},
		},
	}
	err := model.Train(context.Background(), features)
	if err != nil {
		t.Errorf("Train should be no-op and return nil, got error: %v", err)
	}
}

func TestBYOMModel_Predict_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		var req byomRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if req.HorizonSeconds != 1800 {
			t.Errorf("expected horizonSeconds 1800, got %d", req.HorizonSeconds)
		}
		if req.StepSeconds != 60 {
			t.Errorf("expected stepSeconds 60, got %d", req.StepSeconds)
		}
		if len(req.Features) != 3 {
			t.Errorf("expected 3 features, got %d", len(req.Features))
		}

		expectedPredictions := req.HorizonSeconds / req.StepSeconds
		values := make([]float64, expectedPredictions)
		for i := range values {
			values[i] = 100.0 + float64(i)
		}

		resp := byomResponse{
			Metric: "test_forecast",
			Values: values,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	model := NewBYOMModel(server.URL, "test_metric", 60, 1800)
	features := FeatureFrame{
		Rows: []map[string]float64{
			{"ts": 1609459200, "value": 100},
			{"ts": 1609459260, "value": 105},
			{"ts": 1609459320, "value": 110},
		},
	}

	ctx := context.Background()
	forecast, err := model.Predict(ctx, features)
	if err != nil {
		t.Fatalf("Predict failed: %v", err)
	}

	if forecast.Metric != "test_metric" {
		t.Errorf("expected metric 'test_metric', got %q", forecast.Metric)
	}
	if forecast.StepSec != 60 {
		t.Errorf("expected stepSec 60, got %d", forecast.StepSec)
	}
	if forecast.Horizon != 1800 {
		t.Errorf("expected horizon 1800, got %d", forecast.Horizon)
	}

	expectedLen := 30
	if len(forecast.Values) != expectedLen {
		t.Errorf("expected %d values, got %d", expectedLen, len(forecast.Values))
	}

	if forecast.Values[0] != 100.0 {
		t.Errorf("expected first value 100.0, got %f", forecast.Values[0])
	}
}

func TestBYOMModel_Predict_EmptyFeatures(t *testing.T) {
	model := NewBYOMModel("http://localhost:8082/predict", "test_metric", 60, 1800)
	features := FeatureFrame{Rows: []map[string]float64{}}

	_, err := model.Predict(context.Background(), features)
	if err == nil {
		t.Error("expected error for empty features, got nil")
	}
}

func TestBYOMModel_Predict_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	model := NewBYOMModel(server.URL, "test_metric", 60, 1800)
	features := FeatureFrame{
		Rows: []map[string]float64{
			{"ts": 1609459200, "value": 100},
		},
	}

	_, err := model.Predict(context.Background(), features)
	if err == nil {
		t.Error("expected error for HTTP 500, got nil")
	}
}

func TestBYOMModel_Predict_MalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{invalid json"))
	}))
	defer server.Close()

	model := NewBYOMModel(server.URL, "test_metric", 60, 1800)
	features := FeatureFrame{
		Rows: []map[string]float64{
			{"ts": 1609459200, "value": 100},
		},
	}

	_, err := model.Predict(context.Background(), features)
	if err == nil {
		t.Error("expected error for malformed JSON, got nil")
	}
}

func TestBYOMModel_Predict_WrongPredictionCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := byomResponse{
			Metric: "test_forecast",
			Values: []float64{100.0, 105.0},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	model := NewBYOMModel(server.URL, "test_metric", 60, 1800)
	features := FeatureFrame{
		Rows: []map[string]float64{
			{"ts": 1609459200, "value": 100},
		},
	}

	_, err := model.Predict(context.Background(), features)
	if err == nil {
		t.Error("expected error for wrong prediction count, got nil")
	}
}

func TestBYOMModel_Predict_NegativeValues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req byomRequest
		json.NewDecoder(r.Body).Decode(&req)

		expectedPredictions := req.HorizonSeconds / req.StepSeconds
		values := make([]float64, expectedPredictions)
		for i := range values {
			values[i] = -10.0
		}

		resp := byomResponse{
			Metric: "test_forecast",
			Values: values,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	model := NewBYOMModel(server.URL, "test_metric", 60, 1800)
	features := FeatureFrame{
		Rows: []map[string]float64{
			{"ts": 1609459200, "value": 100},
		},
	}

	forecast, err := model.Predict(context.Background(), features)
	if err != nil {
		t.Fatalf("Predict failed: %v", err)
	}

	for i, v := range forecast.Values {
		if v < 0 {
			t.Errorf("prediction[%d] should be clamped to 0, got %f", i, v)
		}
	}
}

func TestBYOMModel_Predict_ContextTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	model := NewBYOMModel(server.URL, "test_metric", 60, 1800)
	features := FeatureFrame{
		Rows: []map[string]float64{
			{"ts": 1609459200, "value": 100},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := model.Predict(ctx, features)
	if err == nil {
		t.Error("expected error for context timeout, got nil")
	}
}
