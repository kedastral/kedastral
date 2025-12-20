// Package main implements the core forecast loop orchestration.
//
// This file contains the Forecaster type which orchestrates the forecast pipeline:
//
//	collect → buildFeatures → predict → calculateReplicas → storeSnapshot
//
// The Forecaster runs continuously via Run(), executing Tick() at regular intervals.
// Each tick performs one complete forecast cycle, updating the stored snapshot that
// the scaler consumes via HTTP API.
//
// The forecast loop is instrumented with Prometheus metrics tracking the duration
// of each pipeline stage (collect, predict, capacity planning) and any errors
// encountered during execution.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/HatiCode/kedastral/cmd/forecaster/metrics"
	"github.com/HatiCode/kedastral/pkg/adapters"
	"github.com/HatiCode/kedastral/pkg/capacity"
	"github.com/HatiCode/kedastral/pkg/features"
	"github.com/HatiCode/kedastral/pkg/models"
	"github.com/HatiCode/kedastral/pkg/storage"
)

// Forecaster orchestrates the forecast loop: collect → predict → plan → store.
type Forecaster struct {
	workload        string
	adapter         adapters.Adapter
	model           models.Model
	builder         *features.Builder
	store           storage.Store
	policy          *capacity.Policy
	horizon         time.Duration
	step            time.Duration
	window          time.Duration
	logger          *slog.Logger
	metrics         *metrics.Metrics
	currentReplicas int
}

// New creates a new Forecaster.
func New(
	workload string,
	adapter adapters.Adapter,
	model models.Model,
	builder *features.Builder,
	store storage.Store,
	policy *capacity.Policy,
	horizon, step, window time.Duration,
	logger *slog.Logger,
	metrics *metrics.Metrics,
) *Forecaster {
	if logger == nil {
		logger = slog.Default()
	}

	return &Forecaster{
		workload:        workload,
		adapter:         adapter,
		model:           model,
		builder:         builder,
		store:           store,
		policy:          policy,
		horizon:         horizon,
		step:            step,
		window:          window,
		logger:          logger,
		metrics:         metrics,
		currentReplicas: policy.MinReplicas,
	}
}

// Run executes the forecast loop at regular intervals.
// Blocks until context is canceled.
func (f *Forecaster) Run(ctx context.Context, interval time.Duration) error {
	f.logger.Info("starting forecast loop", "interval", interval, "window", f.window)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	if err := f.Tick(ctx); err != nil {
		f.logger.Error("initial forecast tick failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			f.logger.Info("forecast loop stopped")
			return ctx.Err()
		case <-ticker.C:
			if err := f.Tick(ctx); err != nil {
				f.logger.Error("forecast tick failed", "error", err)
			}
		}
	}
}

// Tick performs one forecast cycle.
// Exported for testing purposes.
func (f *Forecaster) Tick(ctx context.Context) error {
	start := time.Now()
	f.logger.Debug("starting forecast tick")

	df, collectDuration, err := f.collect(ctx)
	if err != nil {
		if f.metrics != nil {
			f.metrics.RecordError("adapter", "collect_failed")
		}
		return fmt.Errorf("collect: %w", err)
	}

	featureFrame, err := f.buildFeatures(df)
	if err != nil {
		if f.metrics != nil {
			f.metrics.RecordError("features", "build_failed")
		}
		return fmt.Errorf("build features: %w", err)
	}

	// Train the model on historical data to learn patterns
	if err := f.model.Train(ctx, featureFrame); err != nil {
		f.logger.Debug("model training skipped or failed", "error", err)
		// Training is optional for some models (e.g., baseline), so we don't fail here
	}

	forecast, predictDuration, err := f.predict(ctx, featureFrame)
	if err != nil {
		if f.metrics != nil {
			f.metrics.RecordError("model", "predict_failed")
		}
		return fmt.Errorf("predict: %w", err)
	}

	desiredReplicas, capacityDuration := f.calculateReplicas(forecast.Values)

	if err := f.storeSnapshot(forecast, desiredReplicas); err != nil {
		if f.metrics != nil {
			f.metrics.RecordError("store", "put_failed")
		}
		return fmt.Errorf("store: %w", err)
	}

	// Update metrics
	if f.metrics != nil {
		f.metrics.SetForecastAge(0) // Just generated
		f.metrics.SetDesiredReplicas(f.currentReplicas)
		// Set the current predicted value (first forecast point)
		if len(forecast.Values) > 0 {
			f.metrics.SetPredictedValue(forecast.Values[0])
		}
	}

	totalDuration := time.Since(start)
	f.logger.Info("forecast tick complete",
		"workload", f.workload,
		"current_replicas", f.currentReplicas,
		"forecast_points", len(forecast.Values),
		"collect_ms", collectDuration.Milliseconds(),
		"predict_ms", predictDuration.Milliseconds(),
		"capacity_ms", capacityDuration.Milliseconds(),
		"total_ms", totalDuration.Milliseconds(),
	)

	return nil
}

// collect retrieves metrics from the adapter.
func (f *Forecaster) collect(ctx context.Context) (*adapters.DataFrame, time.Duration, error) {
	start := time.Now()

	df, err := f.adapter.Collect(ctx, int(f.window.Seconds()))
	if err != nil {
		return nil, 0, err
	}

	duration := time.Since(start)

	// Record metrics
	if f.metrics != nil {
		f.metrics.RecordCollect(duration.Seconds())
	}

	f.logger.Info("collected metrics",
		"adapter", f.adapter.Name(),
		"rows", len(df.Rows),
		"window_seconds", int(f.window.Seconds()),
		"duration_ms", duration.Milliseconds(),
	)

	return df, duration, nil
}

// buildFeatures converts DataFrame to FeatureFrame.
func (f *Forecaster) buildFeatures(df *adapters.DataFrame) (models.FeatureFrame, error) {
	featureFrame, err := f.builder.BuildFeatures(*df)
	if err != nil {
		return models.FeatureFrame{}, err
	}

	f.logger.Debug("built features", "rows", len(featureFrame.Rows))
	return featureFrame, nil
}

// predict generates forecast using the model.
func (f *Forecaster) predict(ctx context.Context, features models.FeatureFrame) (models.Forecast, time.Duration, error) {
	start := time.Now()

	forecast, err := f.model.Predict(ctx, features)
	if err != nil {
		return models.Forecast{}, 0, err
	}

	duration := time.Since(start)

	// Record metrics
	if f.metrics != nil {
		f.metrics.RecordPredict(duration.Seconds())
	}

	f.logger.Debug("predicted forecast",
		"model", f.model.Name(),
		"values", len(forecast.Values),
		"duration_ms", duration.Milliseconds(),
	)

	return forecast, duration, nil
}

// calculateReplicas converts forecast values to desired replica counts.
func (f *Forecaster) calculateReplicas(values []float64) ([]int, time.Duration) {
	start := time.Now()

	desiredReplicas := capacity.ToReplicas(
		f.currentReplicas,
		values,
		int(f.step.Seconds()),
		*f.policy,
	)

	if len(desiredReplicas) > 0 {
		f.currentReplicas = desiredReplicas[0]
	}

	duration := time.Since(start)

	// Record metrics
	if f.metrics != nil {
		f.metrics.RecordCapacity(duration.Seconds())
	}

	f.logger.Debug("calculated replicas",
		"current", f.currentReplicas,
		"duration_ms", duration.Milliseconds(),
	)

	return desiredReplicas, duration
}

// storeSnapshot persists the forecast snapshot.
func (f *Forecaster) storeSnapshot(forecast models.Forecast, desiredReplicas []int) error {
	snapshot := storage.Snapshot{
		Workload:        f.workload,
		Metric:          forecast.Metric,
		GeneratedAt:     time.Now(),
		StepSeconds:     int(f.step.Seconds()),
		HorizonSeconds:  int(f.horizon.Seconds()),
		Values:          forecast.Values,
		DesiredReplicas: desiredReplicas,
	}

	if err := f.store.Put(snapshot); err != nil {
		return err
	}

	f.logger.Debug("stored snapshot", "workload", f.workload)
	return nil
}

// GetStore returns the underlying store for HTTP handlers.
func (f *Forecaster) GetStore() storage.Store {
	return f.store
}

// GetWorkload returns the workload name.
func (f *Forecaster) GetWorkload() string {
	return f.workload
}
