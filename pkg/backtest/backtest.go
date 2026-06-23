package backtest

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/HatiCode/kedastral/pkg/adapters"
	"github.com/HatiCode/kedastral/pkg/capacity"
	"github.com/HatiCode/kedastral/pkg/features"
	"github.com/HatiCode/kedastral/pkg/models"
)

// Config controls a walk-forward backtest run.
type Config struct {
	// Window is the trailing history each model is trained on.
	Window time.Duration
	// Horizon is how far ahead each forecast predicts.
	Horizon time.Duration
	// Step is the spacing between points; the input series must match it.
	Step time.Duration
	// Stride is how far the evaluation point advances between windows.
	// Defaults to Step when zero.
	Stride time.Duration
	// NewModel builds a fresh model for each window. Using a factory keeps each
	// window's evaluation independent of state left over from the previous one.
	NewModel func() models.Model
	// Policy is the capacity planner policy used to turn forecasts into replicas.
	Policy capacity.Policy
}

// Report summarizes a backtest run.
type Report struct {
	Model                string  `json:"model"`
	Windows              int     `json:"windows"`
	Skipped              int     `json:"skipped"`
	Points               int     `json:"points"`
	MAE                  float64 `json:"mae"`
	RMSE                 float64 `json:"rmse"`
	MAPE                 float64 `json:"mape"`
	UnderProvisionedRate float64 `json:"underProvisionedRate"`
	MeanOverProvision    float64 `json:"meanOverProvision"`
}

// Run performs a walk-forward backtest of cfg over the series.
func Run(ctx context.Context, series Series, cfg Config) (Report, error) {
	if cfg.NewModel == nil {
		return Report{}, fmt.Errorf("NewModel is required")
	}
	if cfg.Step <= 0 {
		return Report{}, fmt.Errorf("step must be > 0")
	}
	if cfg.Horizon < cfg.Step {
		return Report{}, fmt.Errorf("horizon (%v) must be >= step (%v)", cfg.Horizon, cfg.Step)
	}
	if cfg.Window < cfg.Step {
		return Report{}, fmt.Errorf("window (%v) must be >= step (%v)", cfg.Window, cfg.Step)
	}

	stepSec := int(cfg.Step.Seconds())
	windowSteps := int(cfg.Window / cfg.Step)
	horizonSteps := int(cfg.Horizon / cfg.Step)
	strideSteps := int(cfg.Stride / cfg.Step)
	if strideSteps < 1 {
		strideSteps = 1
	}

	if series.Len() < windowSteps+horizonSteps {
		return Report{}, fmt.Errorf("series too short: need at least %d points, got %d", windowSteps+horizonSteps, series.Len())
	}

	builder := features.NewBuilder()
	report := Report{Model: cfg.NewModel().Name()}

	var predicted, actual []float64
	var planned, needed []int
	prevReplicas := cfg.Policy.MinReplicas

	for i := windowSteps; i+horizonSteps <= series.Len(); i += strideSteps {
		frame, err := builder.BuildFeatures(historyDataFrame(series, i-windowSteps, i))
		if err != nil {
			report.Skipped++
			continue
		}

		model := cfg.NewModel()
		// Training failures are non-fatal: some models fall back to a simpler estimate.
		_ = model.Train(ctx, frame)

		forecast, err := model.Predict(ctx, frame)
		if err != nil || len(forecast.Values) == 0 {
			report.Skipped++
			continue
		}

		future := series.Values[i : i+horizonSteps]
		count := min(len(forecast.Values), horizonSteps)

		replicas := capacity.ToReplicas(prevReplicas, forecast.Values, stepSec, cfg.Policy, forecast.Quantiles)

		for k := 0; k < count; k++ {
			predicted = append(predicted, forecast.Values[k])
			actual = append(actual, future[k])
			if k < len(replicas) {
				planned = append(planned, replicas[k])
				needed = append(needed, neededReplicas(future[k], cfg.Policy))
			}
		}

		if len(replicas) > 0 {
			prevReplicas = replicas[0]
		}
		report.Windows++
	}

	if report.Windows == 0 {
		return report, fmt.Errorf("no windows evaluated (all %d forecasts failed); the model may need a larger --window (e.g. >= its seasonal period) or the series may be too short", report.Skipped)
	}

	report.Points = len(predicted)
	report.MAE = MAE(predicted, actual)
	report.RMSE = RMSE(predicted, actual)
	report.MAPE = MAPE(predicted, actual)
	report.UnderProvisionedRate = UnderProvisionedRate(planned, needed)
	report.MeanOverProvision = MeanOverProvision(planned, needed)

	return report, nil
}

// neededReplicas is the bare replica requirement for an actual load value, floored at
// MinReplicas. Headroom is intentionally excluded: it is the planner's safety margin,
// so over-provisioning it causes is what MeanOverProvision is meant to surface.
func neededReplicas(value float64, policy capacity.Policy) int {
	required := 0
	if policy.TargetPerPod > 0 {
		required = int(math.Ceil(value / policy.TargetPerPod))
	}
	if required < policy.MinReplicas {
		required = policy.MinReplicas
	}
	return required
}

func historyDataFrame(series Series, from, to int) adapters.DataFrame {
	rows := make([]adapters.Row, 0, to-from)
	for i := from; i < to; i++ {
		rows = append(rows, adapters.Row{"ts": series.Times[i], "value": series.Values[i]})
	}
	return adapters.DataFrame{Rows: rows}
}
