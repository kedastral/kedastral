package main

import (
	"fmt"
	"log/slog"

	"github.com/HatiCode/kedastral/cmd/forecaster/config"
	"github.com/HatiCode/kedastral/cmd/forecaster/metrics"
	fmodels "github.com/HatiCode/kedastral/cmd/forecaster/models"
	"github.com/HatiCode/kedastral/pkg/adapters"
	"github.com/HatiCode/kedastral/pkg/capacity"
	"github.com/HatiCode/kedastral/pkg/features"
	"github.com/HatiCode/kedastral/pkg/storage"
)

// buildWorkloadForecaster wires an adapter, model, capacity policy, and features
// builder into a WorkloadForecaster for a single workload. It is shared by legacy
// flag-mode startup and the operator controller, which rebuilds forecasters when a
// ForecastPolicy changes.
func buildWorkloadForecaster(wc config.WorkloadConfig, store storage.Store, logger *slog.Logger) (*WorkloadForecaster, error) {
	adapter, err := adapters.New(wc.Adapter, wc.AdapterConfig, int(wc.Step.Seconds()))
	if err != nil {
		return nil, fmt.Errorf("create adapter for workload %q: %w", wc.Name, err)
	}

	model := fmodels.NewForWorkload(wc, logger)
	builder := features.NewBuilder()

	quantileLevel, err := capacity.ParseQuantileLevel(wc.QuantileLevel)
	if err != nil {
		return nil, fmt.Errorf("invalid quantile level %q for workload %q: %w", wc.QuantileLevel, wc.Name, err)
	}
	if quantileLevel > 0 {
		logger.Info("quantile-based capacity planning enabled",
			"workload", wc.Name,
			"quantile", capacity.FormatQuantileLevel(quantileLevel))
	}

	policy := &capacity.Policy{
		TargetPerPod:          wc.TargetPerPod,
		Headroom:              wc.Headroom,
		QuantileLevel:         quantileLevel,
		MinReplicas:           wc.MinReplicas,
		MaxReplicas:           wc.MaxReplicas,
		UpMaxFactorPerStep:    wc.UpMaxFactorPerStep,
		DownMaxPercentPerStep: wc.DownMaxPercentPerStep,
	}

	forecaster := NewWorkloadForecaster(
		wc.Name,
		adapter,
		model,
		builder,
		store,
		policy,
		wc.Horizon,
		wc.Step,
		wc.Window,
		wc.Interval,
		logger,
		metrics.GetOrCreate(wc.Name),
	)

	return forecaster, nil
}
