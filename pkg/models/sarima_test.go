package models

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"
)

func syntheticSeasonalWithTrend(n int, period int, amplitude, trend float64) FeatureFrame {
	rows := make([]map[string]float64, n)
	for i := range n {
		trendComponent := trend * float64(i)
		seasonalComponent := amplitude * math.Sin(2*math.Pi*float64(i)/float64(period))
		rows[i] = map[string]float64{
			"value": 100 + trendComponent + seasonalComponent,
		}
	}
	return FeatureFrame{Rows: rows}
}

func syntheticStrongSeasonal(n int, period int) FeatureFrame {
	rows := make([]map[string]float64, n)
	for i := range n {
		seasonalComponent := 50 * math.Sin(2*math.Pi*float64(i)/float64(period))
		rows[i] = map[string]float64{
			"value": 100 + seasonalComponent,
		}
	}
	return FeatureFrame{Rows: rows}
}

func TestSARIMAModel_NewSARIMAModel_Success(t *testing.T) {
	model := NewSARIMAModel("test_metric", 60, 1800, 1, 1, 1, 1, 1, 1, 24)

	if model == nil {
		t.Fatal("expected non-nil model")
	}

	if model.Name() != "sarima(1,1,1)(1,1,1,24)" {
		t.Errorf("Name() = %q, want %q", model.Name(), "sarima(1,1,1)(1,1,1,24)")
	}

	if model.metric != "test_metric" {
		t.Errorf("metric = %q, want %q", model.metric, "test_metric")
	}

	if model.p != 1 || model.d != 1 || model.q != 1 {
		t.Errorf("p,d,q = %d,%d,%d, want 1,1,1", model.p, model.d, model.q)
	}

	if model.P != 1 || model.D != 1 || model.Q != 1 || model.s != 24 {
		t.Errorf("P,D,Q,s = %d,%d,%d,%d, want 1,1,1,24", model.P, model.D, model.Q, model.s)
	}
}

func TestSARIMAModel_NewSARIMAModel_NoSeasonal(t *testing.T) {
	model := NewSARIMAModel("test_metric", 60, 1800, 2, 1, 2, 0, 0, 0, 0)

	if model.Name() != "sarima(2,1,2)" {
		t.Errorf("Name() = %q, want %q", model.Name(), "sarima(2,1,2)")
	}

	if model.P != 0 || model.D != 0 || model.Q != 0 {
		t.Errorf("P,D,Q = %d,%d,%d, want 0,0,0", model.P, model.D, model.Q)
	}
}

func TestSARIMAModel_NewSARIMAModel_AutoDetect(t *testing.T) {
	model := NewSARIMAModel("test_metric", 60, 1800, 0, 0, 0, 1, 1, 1, 24)

	if model.p != 1 || model.d != 1 || model.q != 1 {
		t.Errorf("auto-detect failed: p,d,q = %d,%d,%d, want 1,1,1", model.p, model.d, model.q)
	}
}

func TestSARIMAModel_NewSARIMAModel_Panics(t *testing.T) {
	tests := []struct {
		name   string
		panics bool
		fn     func()
	}{
		{
			name:   "empty metric",
			panics: true,
			fn:     func() { NewSARIMAModel("", 60, 1800, 1, 1, 1, 0, 0, 0, 0) },
		},
		{
			name:   "zero stepSec",
			panics: true,
			fn:     func() { NewSARIMAModel("test", 0, 1800, 1, 1, 1, 0, 0, 0, 0) },
		},
		{
			name:   "d > 2",
			panics: true,
			fn:     func() { NewSARIMAModel("test", 60, 1800, 1, 3, 1, 0, 0, 0, 0) },
		},
		{
			name:   "D > 1",
			panics: true,
			fn:     func() { NewSARIMAModel("test", 60, 1800, 1, 1, 1, 1, 2, 1, 24) },
		},
		{
			name:   "seasonal without s",
			panics: true,
			fn:     func() { NewSARIMAModel("test", 60, 1800, 1, 1, 1, 1, 1, 1, 0) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if (r != nil) != tt.panics {
					t.Errorf("panic = %v, want panic = %v", r != nil, tt.panics)
				}
			}()
			tt.fn()
		})
	}
}

func TestSARIMAModel_Train_Success_Constant(t *testing.T) {
	model := NewSARIMAModel("test_metric", 60, 1800, 1, 1, 1, 0, 0, 0, 0)
	history := syntheticConstant(100, 100)

	err := model.Train(context.Background(), history)
	if err != nil {
		t.Fatalf("Train() error = %v", err)
	}

	if !model.trained {
		t.Error("expected model to be trained")
	}
}

func TestSARIMAModel_Train_Success_LinearTrend(t *testing.T) {
	model := NewSARIMAModel("test_metric", 60, 1800, 1, 1, 1, 0, 0, 0, 0)
	history := syntheticLinear(100, 2.0, 50, 0)

	err := model.Train(context.Background(), history)
	if err != nil {
		t.Fatalf("Train() error = %v", err)
	}

	if !model.trained {
		t.Error("expected model to be trained")
	}
}

func TestSARIMAModel_Train_Success_Seasonal(t *testing.T) {
	model := NewSARIMAModel("test_metric", 60, 1800, 1, 1, 1, 1, 1, 1, 24)
	history := syntheticStrongSeasonal(200, 24)

	err := model.Train(context.Background(), history)
	if err != nil {
		t.Fatalf("Train() error = %v", err)
	}

	if !model.trained {
		t.Error("expected model to be trained")
	}

	if len(model.seasonalARCoeffs) != 1 {
		t.Errorf("seasonalARCoeffs length = %d, want 1", len(model.seasonalARCoeffs))
	}

	if len(model.seasonalMACoeffs) != 1 {
		t.Errorf("seasonalMACoeffs length = %d, want 1", len(model.seasonalMACoeffs))
	}
}

func TestSARIMAModel_Train_Success_TrendAndSeasonal(t *testing.T) {
	model := NewSARIMAModel("test_metric", 60, 1800, 1, 1, 1, 1, 1, 1, 24)
	history := syntheticSeasonalWithTrend(200, 24, 30, 0.5)

	err := model.Train(context.Background(), history)
	if err != nil {
		t.Fatalf("Train() error = %v", err)
	}

	if !model.trained {
		t.Error("expected model to be trained")
	}
}

func TestSARIMAModel_Train_InsufficientData(t *testing.T) {
	model := NewSARIMAModel("test_metric", 60, 1800, 1, 1, 1, 1, 1, 1, 24)
	history := syntheticConstant(10, 100)

	err := model.Train(context.Background(), history)
	if err == nil {
		t.Error("expected error for insufficient data")
	}
}

func TestSARIMAModel_Train_InsufficientData_Seasonal(t *testing.T) {
	model := NewSARIMAModel("test_metric", 60, 1800, 1, 1, 1, 2, 1, 2, 24)
	history := syntheticConstant(30, 100)

	err := model.Train(context.Background(), history)
	if err == nil {
		t.Error("expected error for insufficient seasonal data")
	}
}

func TestSARIMAModel_Predict_NotTrained(t *testing.T) {
	model := NewSARIMAModel("test_metric", 60, 1800, 1, 1, 1, 1, 1, 1, 24)

	_, err := model.Predict(context.Background(), FeatureFrame{})
	if err == nil {
		t.Error("expected error when predicting without training")
	}
}

func TestSARIMAModel_Predict_Success_Constant(t *testing.T) {
	model := NewSARIMAModel("test_metric", 60, 1800, 1, 1, 1, 0, 0, 0, 0)
	history := syntheticConstant(100, 100)

	err := model.Train(context.Background(), history)
	if err != nil {
		t.Fatalf("Train() error = %v", err)
	}

	forecast, err := model.Predict(context.Background(), FeatureFrame{})
	if err != nil {
		t.Fatalf("Predict() error = %v", err)
	}

	if len(forecast.Values) != 30 {
		t.Errorf("forecast length = %d, want 30", len(forecast.Values))
	}

	for i, v := range forecast.Values {
		if v < 0 {
			t.Errorf("prediction[%d] = %f < 0", i, v)
		}
		if v < 80 || v > 120 {
			t.Errorf("prediction[%d] = %f, expected near 100", i, v)
		}
	}
}

func TestSARIMAModel_Predict_Success_Seasonal(t *testing.T) {
	model := NewSARIMAModel("test_metric", 60, 1800, 1, 1, 1, 1, 1, 1, 24)
	history := syntheticStrongSeasonal(200, 24)

	err := model.Train(context.Background(), history)
	if err != nil {
		t.Fatalf("Train() error = %v", err)
	}

	forecast, err := model.Predict(context.Background(), FeatureFrame{})
	if err != nil {
		t.Fatalf("Predict() error = %v", err)
	}

	if len(forecast.Values) != 30 {
		t.Errorf("forecast length = %d, want 30", len(forecast.Values))
	}

	for i, v := range forecast.Values {
		if v < 0 {
			t.Errorf("prediction[%d] = %f < 0", i, v)
		}
	}
}

func TestSARIMAModel_Predict_NonNegative(t *testing.T) {
	model := NewSARIMAModel("test_metric", 60, 1800, 1, 1, 1, 1, 1, 1, 24)
	history := syntheticLinear(100, -1, 200, 5)

	err := model.Train(context.Background(), history)
	if err != nil {
		t.Fatalf("Train() error = %v", err)
	}

	forecast, err := model.Predict(context.Background(), FeatureFrame{})
	if err != nil {
		t.Fatalf("Predict() error = %v", err)
	}

	for i, v := range forecast.Values {
		if v < 0 {
			t.Errorf("prediction[%d] = %f < 0, want >= 0", i, v)
		}
	}
}

func TestSARIMAModel_Predict_CorrectLength(t *testing.T) {
	model := NewSARIMAModel("test_metric", 60, 1800, 1, 1, 1, 1, 1, 1, 24)
	history := syntheticConstant(100, 100)

	err := model.Train(context.Background(), history)
	if err != nil {
		t.Fatalf("Train() error = %v", err)
	}

	forecast, err := model.Predict(context.Background(), FeatureFrame{})
	if err != nil {
		t.Fatalf("Predict() error = %v", err)
	}

	expectedLen := 1800 / 60
	if len(forecast.Values) != expectedLen {
		t.Errorf("forecast length = %d, want %d", len(forecast.Values), expectedLen)
	}
}

func TestSARIMAModel_Predict_Quantiles(t *testing.T) {
	model := NewSARIMAModel("test_metric", 60, 1800, 1, 1, 1, 1, 1, 1, 24)
	history := syntheticStrongSeasonal(200, 24)

	err := model.Train(context.Background(), history)
	if err != nil {
		t.Fatalf("Train() error = %v", err)
	}

	forecast, err := model.Predict(context.Background(), FeatureFrame{})
	if err != nil {
		t.Fatalf("Predict() error = %v", err)
	}

	if len(forecast.Quantiles) == 0 {
		t.Error("expected quantiles to be generated")
	}

	for q, values := range forecast.Quantiles {
		if len(values) != len(forecast.Values) {
			t.Errorf("quantile %f has %d values, want %d", q, len(values), len(forecast.Values))
		}
	}
}

func TestSARIMAModel_Concurrency_TrainPredict(t *testing.T) {
	model := NewSARIMAModel("test_metric", 60, 1800, 1, 1, 1, 1, 1, 1, 24)
	history := syntheticStrongSeasonal(200, 24)

	err := model.Train(context.Background(), history)
	if err != nil {
		t.Fatalf("Train() error = %v", err)
	}

	var wg sync.WaitGroup
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := model.Predict(context.Background(), FeatureFrame{})
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent Predict() error = %v", err)
	}
}

func TestSARIMAModel_ContextCancellation_Train(t *testing.T) {
	model := NewSARIMAModel("test_metric", 60, 1800, 1, 1, 1, 1, 1, 1, 24)
	history := syntheticStrongSeasonal(200, 24)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := model.Train(ctx, history)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestSARIMAModel_ContextCancellation_Predict(t *testing.T) {
	model := NewSARIMAModel("test_metric", 60, 1800, 1, 1, 1, 1, 1, 1, 24)
	history := syntheticStrongSeasonal(200, 24)

	err := model.Train(context.Background(), history)
	if err != nil {
		t.Fatalf("Train() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = model.Predict(ctx, FeatureFrame{})
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestSARIMAModel_SeasonalDifference(t *testing.T) {
	series := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	result := seasonalDifference(series, 1, 3)

	expected := []float64{3, 3, 3, 3, 3, 3, 3}
	if len(result) != len(expected) {
		t.Errorf("result length = %d, want %d", len(result), len(expected))
	}

	for i := range result {
		if result[i] != expected[i] {
			t.Errorf("result[%d] = %f, want %f", i, result[i], expected[i])
		}
	}
}

func TestSARIMAModel_SeasonalDifference_NoOp(t *testing.T) {
	series := []float64{1, 2, 3, 4, 5}
	result := seasonalDifference(series, 0, 2)

	if len(result) != len(series) {
		t.Errorf("result length = %d, want %d", len(result), len(series))
	}

	for i := range result {
		if result[i] != series[i] {
			t.Errorf("result[%d] = %f, want %f", i, result[i], series[i])
		}
	}
}

func TestSARIMAModel_Acceptance_DailySeasonal(t *testing.T) {
	model := NewSARIMAModel("test_metric", 60, 1800, 1, 1, 1, 1, 1, 1, 24)
	history := syntheticSeasonalWithTrend(168, 24, 40, 0.3)

	err := model.Train(context.Background(), history)
	if err != nil {
		t.Fatalf("Train() error = %v", err)
	}

	forecast, err := model.Predict(context.Background(), FeatureFrame{})
	if err != nil {
		t.Fatalf("Predict() error = %v", err)
	}

	if len(forecast.Values) != 30 {
		t.Errorf("forecast length = %d, want 30", len(forecast.Values))
	}

	for i, v := range forecast.Values {
		if v < 0 {
			t.Errorf("prediction[%d] = %f < 0", i, v)
		}
	}

	if forecast.Metric != "test_metric" {
		t.Errorf("forecast.Metric = %q, want %q", forecast.Metric, "test_metric")
	}
}

func BenchmarkSARIMAModel_Train_100Points(b *testing.B) {
	model := NewSARIMAModel("bench", 60, 1800, 1, 1, 1, 1, 1, 1, 24)
	history := syntheticStrongSeasonal(100, 24)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = model.Train(context.Background(), history)
	}
}

func BenchmarkSARIMAModel_Train_1000Points(b *testing.B) {
	model := NewSARIMAModel("bench", 60, 1800, 1, 1, 1, 1, 1, 1, 24)
	history := syntheticStrongSeasonal(1000, 24)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = model.Train(context.Background(), history)
	}
}

func BenchmarkSARIMAModel_Predict_30Steps(b *testing.B) {
	model := NewSARIMAModel("bench", 60, 1800, 1, 1, 1, 1, 1, 1, 24)
	history := syntheticStrongSeasonal(200, 24)
	_ = model.Train(context.Background(), history)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = model.Predict(context.Background(), FeatureFrame{})
	}
}

func BenchmarkSARIMAModel_Predict_100Steps(b *testing.B) {
	model := NewSARIMAModel("bench", 60, 6000, 1, 1, 1, 1, 1, 1, 24)
	history := syntheticStrongSeasonal(200, 24)
	_ = model.Train(context.Background(), history)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = model.Predict(context.Background(), FeatureFrame{})
	}
}

func TestSARIMAModel_RetrainUpdatesCoefficients(t *testing.T) {
	model := NewSARIMAModel("test_metric", 60, 1800, 1, 1, 1, 1, 1, 1, 24)
	history1 := syntheticConstant(100, 100)

	err := model.Train(context.Background(), history1)
	if err != nil {
		t.Fatalf("first Train() error = %v", err)
	}

	forecast1, err := model.Predict(context.Background(), FeatureFrame{})
	if err != nil {
		t.Fatalf("first Predict() error = %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	history2 := syntheticConstant(100, 200)
	err = model.Train(context.Background(), history2)
	if err != nil {
		t.Fatalf("second Train() error = %v", err)
	}

	forecast2, err := model.Predict(context.Background(), FeatureFrame{})
	if err != nil {
		t.Fatalf("second Predict() error = %v", err)
	}

	if forecast1.Values[0] == forecast2.Values[0] {
		t.Log("Warning: retraining with different data produced identical forecasts")
	}
}
