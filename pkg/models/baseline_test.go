package models

import (
	"context"
	"testing"
)

func TestBaselineModel_Name(t *testing.T) {
	model := NewBaselineModel("test_metric", 60, 1800)
	if got := model.Name(); got != "baseline" {
		t.Errorf("Name() = %q, want %q", got, "baseline")
	}
}

func TestBaselineModel_Predict_Basic(t *testing.T) {
	tests := []struct {
		name        string
		metric      string
		stepSec     int
		horizon     int
		features    FeatureFrame
		wantLen     int
		wantErr     bool
		checkValues func(t *testing.T, values []float64)
	}{
		{
			name:     "simple constant series",
			metric:   "http_rps",
			stepSec:  60,
			horizon:  300, // 5 minutes
			features: makeFeatureFrame([]float64{100, 100, 100, 100, 100}),
			wantLen:  5, // 300/60 = 5 steps
			wantErr:  false,
			checkValues: func(t *testing.T, values []float64) {
				// All predictions should be close to 100
				for i, v := range values {
					if v < 90 || v > 110 {
						t.Errorf("value[%d] = %.2f, want ~100", i, v)
					}
				}
			},
		},
		{
			name:     "empty features",
			metric:   "http_rps",
			stepSec:  60,
			horizon:  300,
			features: FeatureFrame{Rows: []map[string]float64{}},
			wantLen:  0,
			wantErr:  true,
		},
		{
			name:    "features without value field",
			metric:  "http_rps",
			stepSec: 60,
			horizon: 300,
			features: FeatureFrame{
				Rows: []map[string]float64{
					{"timestamp": 1000},
					{"timestamp": 2000},
				},
			},
			wantLen: 0,
			wantErr: true,
		},
		{
			name:     "single point",
			metric:   "http_rps",
			stepSec:  60,
			horizon:  180,
			features: makeFeatureFrame([]float64{150}),
			wantLen:  3, // 180/60 = 3
			wantErr:  false,
			checkValues: func(t *testing.T, values []float64) {
				// Forecast should be constant at ~150
				for i, v := range values {
					if v < 140 || v > 160 {
						t.Errorf("value[%d] = %.2f, want ~150", i, v)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewBaselineModel(tt.metric, tt.stepSec, tt.horizon)
			forecast, err := model.Predict(context.Background(), tt.features)

			if (err != nil) != tt.wantErr {
				t.Errorf("Predict() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				return // Expected error, test passed
			}

			if len(forecast.Values) != tt.wantLen {
				t.Errorf("len(Values) = %d, want %d", len(forecast.Values), tt.wantLen)
			}

			if forecast.Metric != tt.metric {
				t.Errorf("Metric = %q, want %q", forecast.Metric, tt.metric)
			}

			if forecast.StepSec != tt.stepSec {
				t.Errorf("StepSec = %d, want %d", forecast.StepSec, tt.stepSec)
			}

			if forecast.Horizon != tt.horizon {
				t.Errorf("Horizon = %d, want %d", forecast.Horizon, tt.horizon)
			}

			if tt.checkValues != nil {
				tt.checkValues(t, forecast.Values)
			}
		})
	}
}

// TestBaselineModel_Predict_AcceptanceTest verifies predictive behavior:
// GIVEN: Monotonically increasing series 100..200
// EXPECT: Predictions continue the upward trend and are non-decreasing
func TestBaselineModel_Predict_AcceptanceTest(t *testing.T) {
	// Create monotonically increasing series: 100, 105, 110, ..., 200
	// That's 21 points (100 to 200 in steps of 5)
	values := make([]float64, 21)
	for i := range values {
		values[i] = 100 + float64(i*5)
	}

	model := NewBaselineModel("http_rps", 60, 1800) // 30m horizon
	features := makeFeatureFrame(values)

	forecast, err := model.Predict(context.Background(), features)
	if err != nil {
		t.Fatalf("Predict() error = %v", err)
	}

	lastInput := values[len(values)-1] // 200

	// With upward trend, first prediction should be >= last input
	if forecast.Values[0] < lastInput {
		t.Errorf("forecast[0] = %.2f < %.2f (last input), want >= last (continuing trend)",
			forecast.Values[0], lastInput)
	}

	// Predictions should continue the trend but not explode unreasonably
	// With a slope of 5 per minute, after 30 minutes we'd expect ~350 max
	for i, v := range forecast.Values {
		if v > lastInput*2.0 {
			t.Errorf("value[%d] = %.2f > %.2f (2x last input), unreasonably high", i, v, lastInput*2.0)
		}
		if v < 0 {
			t.Errorf("value[%d] = %.2f is negative", i, v)
		}
	}

	// Check predictions are generally non-decreasing (allowing small variations)
	decreaseCount := 0
	for i := 1; i < len(forecast.Values); i++ {
		if forecast.Values[i] < forecast.Values[i-1]-1.0 { // allow 1.0 tolerance
			decreaseCount++
		}
	}
	if decreaseCount > len(forecast.Values)/10 {
		t.Errorf("too many decreasing predictions: %d out of %d", decreaseCount, len(forecast.Values))
	}
}

func TestBaselineModel_Predict_NonNegative(t *testing.T) {
	// Test that predictions are always non-negative, even with edge cases
	tests := []struct {
		name   string
		values []float64
	}{
		{
			name:   "all positive",
			values: []float64{10, 20, 30, 40, 50},
		},
		{
			name:   "with zeros",
			values: []float64{0, 0, 10, 20, 30},
		},
		{
			name:   "very small values",
			values: []float64{0.1, 0.2, 0.3, 0.4, 0.5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewBaselineModel("test", 60, 600)
			features := makeFeatureFrame(tt.values)

			forecast, err := model.Predict(context.Background(), features)
			if err != nil {
				t.Fatalf("Predict() error = %v", err)
			}

			for i, v := range forecast.Values {
				if v < 0 {
					t.Errorf("value[%d] = %.2f is negative, want non-negative", i, v)
				}
			}
		})
	}
}

func TestBaselineModel_Train_Seasonality(t *testing.T) {
	model := NewBaselineModel("http_rps", 60, 1800)

	// Create training data with clear hourly patterns
	// Hour 9: high traffic (~200), Hour 14: moderate (~150), Hour 22: low (~100)
	history := FeatureFrame{
		Rows: []map[string]float64{
			{"value": 200, "hour": 9},
			{"value": 210, "hour": 9},
			{"value": 190, "hour": 9},
			{"value": 150, "hour": 14},
			{"value": 160, "hour": 14},
			{"value": 140, "hour": 14},
			{"value": 100, "hour": 22},
			{"value": 110, "hour": 22},
			{"value": 90, "hour": 22},
		},
	}

	err := model.Train(context.Background(), history)
	if err != nil {
		t.Fatalf("Train() error = %v", err)
	}

	// Verify seasonality was learned
	if len(model.hourSeasonality) == 0 {
		t.Error("expected hourSeasonality to be populated after training")
	}

	// Check specific hours
	if pattern9, ok := model.hourSeasonality[9]; ok {
		want := 200.0 // (200+210+190)/3
		if pattern9.mean < 195 || pattern9.mean > 205 {
			t.Errorf("hourSeasonality[9].mean = %.2f, want ~%.2f", pattern9.mean, want)
		}
		if pattern9.max != 210 {
			t.Errorf("hourSeasonality[9].max = %.2f, want 210", pattern9.max)
		}
		if pattern9.min != 190 {
			t.Errorf("hourSeasonality[9].min = %.2f, want 190", pattern9.min)
		}
	} else {
		t.Error("expected seasonality for hour 9")
	}
}

func TestBaselineModel_Train_EmptyHistory(t *testing.T) {
	model := NewBaselineModel("http_rps", 60, 1800)

	// Training with empty history should not error
	err := model.Train(context.Background(), FeatureFrame{})
	if err != nil {
		t.Errorf("Train() with empty history error = %v, want nil", err)
	}
}

func TestBaselineModel_Predict_WithRecurringSpikes(t *testing.T) {
	// Test the model's ability to predict recurring spikes (e.g., every 30 minutes)
	// This simulates 3 hours of data with spikes at minute 0 and 30 of each hour

	model := NewBaselineModel("http_rps", 60, 1800) // 30min horizon, 1min steps

	// Create 3 hours of training data with spikes every 30 minutes
	// Baseline load: 100 RPS, Spike load: 500 RPS
	var trainingData FeatureFrame
	for hour := range 3 {
		for minute := range 60 {
			value := 100.0 // baseline
			if minute == 0 || minute == 30 {
				value = 500.0 // spike
			}
			trainingData.Rows = append(trainingData.Rows, map[string]float64{
				"value":  value,
				"hour":   float64(hour),
				"minute": float64(minute),
			})
		}
	}

	// Train the model
	err := model.Train(context.Background(), trainingData)
	if err != nil {
		t.Fatalf("Train() error = %v", err)
	}

	// Verify it learned the spike pattern
	if pattern, ok := model.minuteSeasonality[0]; ok {
		if pattern.mean < 450 {
			t.Errorf("minuteSeasonality[0].mean = %.2f, expected ~500 (spike)", pattern.mean)
		}
	} else {
		t.Error("expected minute 0 pattern to be learned")
	}

	// Now predict: we're at minute 20 (baseline load), predict next 30 minutes
	// The model should predict an upcoming spike at minute 30
	currentFeatures := FeatureFrame{
		Rows: []map[string]float64{
			{"value": 95, "minute": 17, "hour": 3},
			{"value": 100, "minute": 18, "hour": 3},
			{"value": 105, "minute": 19, "hour": 3},
			{"value": 100, "minute": 20, "hour": 3},
		},
	}

	forecast, err := model.Predict(context.Background(), currentFeatures)
	if err != nil {
		t.Fatalf("Predict() error = %v", err)
	}

	// At 10 minutes ahead (minute 30), we should predict a spike
	// That's index 9 (10 minutes / 60 seconds per step)
	spikeIndex := 9 // 10 minutes ahead
	if forecast.Values[spikeIndex] < 300 {
		t.Errorf("forecast at minute 30 = %.2f, expected > 300 (spike prediction)", forecast.Values[spikeIndex])
	}

	// Earlier predictions (minute 21-29) should be lower than the spike
	for i := range spikeIndex {
		if forecast.Values[i] > forecast.Values[spikeIndex]*0.8 {
			t.Errorf("forecast[%d] = %.2f should be significantly lower than spike forecast %.2f",
				i, forecast.Values[i], forecast.Values[spikeIndex])
		}
	}
}

func TestBaselineModel_Predict_TrendDetection(t *testing.T) {
	// Test that the model detects and projects upward trends

	model := NewBaselineModel("http_rps", 60, 600) // 10min horizon

	// Create steadily increasing load: 100, 110, 120, 130, 140, 150
	increasingLoad := FeatureFrame{
		Rows: []map[string]float64{
			{"value": 100, "minute": 0},
			{"value": 110, "minute": 1},
			{"value": 120, "minute": 2},
			{"value": 130, "minute": 3},
			{"value": 140, "minute": 4},
			{"value": 150, "minute": 5},
		},
	}

	forecast, err := model.Predict(context.Background(), increasingLoad)
	if err != nil {
		t.Fatalf("Predict() error = %v", err)
	}

	// Forecast should continue the upward trend
	// First prediction should be > 150 (last value)
	if forecast.Values[0] <= 150 {
		t.Errorf("forecast[0] = %.2f, expected > 150 (continuing trend)", forecast.Values[0])
	}

	// Predictions should generally be increasing (may plateau)
	// Check that later predictions are >= first prediction
	for i := 1; i < len(forecast.Values); i++ {
		if forecast.Values[i] < forecast.Values[0]*0.9 {
			t.Errorf("forecast[%d] = %.2f dropped significantly from forecast[0] = %.2f",
				i, forecast.Values[i], forecast.Values[0])
		}
	}
}

// makeFeatureFrame is a helper to create a FeatureFrame from a slice of values
func makeFeatureFrame(values []float64) FeatureFrame {
	rows := make([]map[string]float64, len(values))
	for i, v := range values {
		rows[i] = map[string]float64{
			"value":     v,
			"timestamp": float64(i * 60), // 1 minute intervals
		}
	}
	return FeatureFrame{Rows: rows}
}
