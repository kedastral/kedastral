package backtest

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/HatiCode/kedastral/pkg/capacity"
	"github.com/HatiCode/kedastral/pkg/models"
)

func syntheticSeries(points int, step time.Duration) Series {
	series := Series{}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < points; i++ {
		series.Times = append(series.Times, start.Add(time.Duration(i)*step))
		series.Values = append(series.Values, 100+50*math.Sin(float64(i)/10))
	}
	return series
}

func baselineConfig() Config {
	return Config{
		Window:  60 * time.Minute,
		Horizon: 10 * time.Minute,
		Step:    time.Minute,
		NewModel: func() models.Model {
			return models.NewBaselineModel("value", 60, 600)
		},
		Policy: capacity.Policy{
			TargetPerPod: 50,
			Headroom:     1.2,
			MinReplicas:  1,
			MaxReplicas:  20,
		},
	}
}

func TestRun_Synthetic(t *testing.T) {
	series := syntheticSeries(180, time.Minute)

	report, err := Run(context.Background(), series, baselineConfig())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if report.Windows == 0 {
		t.Fatal("expected at least one evaluated window")
	}
	if report.Points == 0 {
		t.Fatal("expected forecast points")
	}
	if report.Model != "baseline" {
		t.Errorf("Model = %q, want baseline", report.Model)
	}
	for name, v := range map[string]float64{
		"MAE": report.MAE, "RMSE": report.RMSE, "MAPE": report.MAPE,
		"under": report.UnderProvisionedRate, "over": report.MeanOverProvision,
	} {
		if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			t.Errorf("%s metric invalid: %v", name, v)
		}
	}
}

func TestRun_Validation(t *testing.T) {
	series := syntheticSeries(180, time.Minute)

	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{"nil NewModel", func(c *Config) { c.NewModel = nil }},
		{"zero step", func(c *Config) { c.Step = 0 }},
		{"horizon below step", func(c *Config) { c.Horizon = 30 * time.Second }},
		{"window below step", func(c *Config) { c.Window = 30 * time.Second }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baselineConfig()
			tt.mutate(&cfg)
			if _, err := Run(context.Background(), series, cfg); err == nil {
				t.Errorf("expected error for %s", tt.name)
			}
		})
	}
}

func TestRun_SeriesTooShort(t *testing.T) {
	series := syntheticSeries(10, time.Minute) // window+horizon = 70 steps
	if _, err := Run(context.Background(), series, baselineConfig()); err == nil {
		t.Error("expected error for too-short series")
	}
}
