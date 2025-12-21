// Package main implements the core forecast loop orchestration.
//
// This file contains the WorkloadForecaster and MultiForecaster types which orchestrate
// the forecast pipeline for one or more workloads concurrently:
//
//	collect → buildFeatures → predict → calculateReplicas → storeSnapshot
//
// WorkloadForecaster manages forecasting for a single workload with isolated state.
// MultiForecaster coordinates multiple WorkloadForecasters running in parallel goroutines.
//
// Each workload forecaster runs independently with panic recovery and per-operation timeouts
// to ensure one failing workload does not affect others. The forecast loop is instrumented
// with Prometheus metrics tracking the duration of each pipeline stage and any errors.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/HatiCode/kedastral/cmd/forecaster/metrics"
	"github.com/HatiCode/kedastral/pkg/adapters"
	"github.com/HatiCode/kedastral/pkg/capacity"
	"github.com/HatiCode/kedastral/pkg/features"
	"github.com/HatiCode/kedastral/pkg/models"
	"github.com/HatiCode/kedastral/pkg/storage"
)

// WorkloadForecaster orchestrates the forecast loop for a single workload.
type WorkloadForecaster struct {
	name            string
	adapter         adapters.Adapter
	model           models.Model
	builder         *features.Builder
	store           storage.Store
	policy          *capacity.Policy
	horizon         time.Duration
	step            time.Duration
	window          time.Duration
	interval        time.Duration
	logger          *slog.Logger
	metrics         *metrics.Metrics
	currentReplicas int
}

// MultiForecaster manages multiple workload forecasters running concurrently.
type MultiForecaster struct {
	workloads map[string]*WorkloadForecaster
	store     storage.Store
	logger    *slog.Logger
}

// NewWorkloadForecaster creates a new forecaster for a single workload.
func NewWorkloadForecaster(
	name string,
	adapter adapters.Adapter,
	model models.Model,
	builder *features.Builder,
	store storage.Store,
	policy *capacity.Policy,
	horizon, step, window, interval time.Duration,
	logger *slog.Logger,
	m *metrics.Metrics,
) *WorkloadForecaster {
	if logger == nil {
		logger = slog.Default()
	}

	return &WorkloadForecaster{
		name:            name,
		adapter:         adapter,
		model:           model,
		builder:         builder,
		store:           store,
		policy:          policy,
		horizon:         horizon,
		step:            step,
		window:          window,
		interval:        interval,
		logger:          logger.With("workload", name),
		metrics:         m,
		currentReplicas: policy.MinReplicas,
	}
}

// NewMultiForecaster creates a new multi-workload forecaster.
func NewMultiForecaster(workloads []*WorkloadForecaster, store storage.Store, logger *slog.Logger) *MultiForecaster {
	if logger == nil {
		logger = slog.Default()
	}

	wm := make(map[string]*WorkloadForecaster)
	for _, wf := range workloads {
		wm[wf.name] = wf
	}

	return &MultiForecaster{
		workloads: wm,
		store:     store,
		logger:    logger,
	}
}

// Run executes all workload forecasters concurrently until context is canceled.
func (mf *MultiForecaster) Run(ctx context.Context) error {
	if len(mf.workloads) == 0 {
		return fmt.Errorf("no workloads configured")
	}

	mf.logger.Info("starting multi-workload forecaster", "workloads", len(mf.workloads))

	var wg sync.WaitGroup
	errCh := make(chan error, len(mf.workloads))

	for name, wf := range mf.workloads {
		wg.Add(1)
		go func(name string, wf *WorkloadForecaster) {
			defer wg.Done()
			if err := wf.Run(ctx); err != nil && err != context.Canceled {
				errCh <- fmt.Errorf("workload %q: %w", name, err)
			}
		}(name, wf)
	}

	wg.Wait()
	close(errCh)

	var firstErr error
	for err := range errCh {
		if firstErr == nil {
			firstErr = err
		}
		mf.logger.Error("workload forecaster error", "error", err)
	}

	mf.logger.Info("multi-workload forecaster stopped")
	return firstErr
}

// GetStore returns the shared storage backend.
func (mf *MultiForecaster) GetStore() storage.Store {
	return mf.store
}

// Run executes the forecast loop for this workload with panic recovery and graceful shutdown.
func (wf *WorkloadForecaster) Run(ctx context.Context) error {
	defer func() {
		if r := recover(); r != nil {
			wf.logger.Error("workload forecaster panic recovered", "panic", r)
		}
	}()

	wf.logger.Info("starting workload forecaster", "interval", wf.interval, "window", wf.window)

	ticker := time.NewTicker(wf.interval)
	defer ticker.Stop()

	tickCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	if err := wf.tick(tickCtx); err != nil {
		wf.logger.Error("initial forecast tick failed", "error", err)
	}
	cancel()

	for {
		select {
		case <-ctx.Done():
			wf.logger.Info("workload forecaster stopped")
			return ctx.Err()
		case <-ticker.C:
			tickCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			if err := wf.tick(tickCtx); err != nil {
				wf.logger.Error("forecast tick failed", "error", err)
			}
			cancel()
		}
	}
}

// tick performs one forecast cycle with per-operation timeouts.
func (wf *WorkloadForecaster) tick(ctx context.Context) error {
	start := time.Now()
	wf.logger.Debug("starting forecast tick")

	collectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	df, collectDuration, err := wf.collect(collectCtx)
	cancel()
	if err != nil {
		if wf.metrics != nil {
			wf.metrics.RecordError("adapter", "collect_failed")
		}
		return fmt.Errorf("collect: %w", err)
	}

	featureFrame, err := wf.buildFeatures(df)
	if err != nil {
		if wf.metrics != nil {
			wf.metrics.RecordError("features", "build_failed")
		}
		return fmt.Errorf("build features: %w", err)
	}

	trainCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	if err := wf.model.Train(trainCtx, featureFrame); err != nil {
		wf.logger.Debug("model training skipped or failed", "error", err)
	}
	cancel()

	predictCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	forecast, predictDuration, err := wf.predict(predictCtx, featureFrame)
	cancel()
	if err != nil {
		if wf.metrics != nil {
			wf.metrics.RecordError("model", "predict_failed")
		}
		return fmt.Errorf("predict: %w", err)
	}

	desiredReplicas, capacityDuration := wf.calculateReplicas(forecast.Values, forecast.Quantiles)

	storeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	err = wf.storeSnapshot(storeCtx, forecast, desiredReplicas)
	cancel()
	if err != nil {
		if wf.metrics != nil {
			wf.metrics.RecordError("store", "put_failed")
		}
		return fmt.Errorf("store: %w", err)
	}

	if wf.metrics != nil {
		wf.metrics.SetForecastAge(0)
		wf.metrics.SetDesiredReplicas(wf.currentReplicas)
		if len(forecast.Values) > 0 {
			wf.metrics.SetPredictedValue(forecast.Values[0])
		}
	}

	totalDuration := time.Since(start)
	wf.logger.Info("forecast tick complete",
		"current_replicas", wf.currentReplicas,
		"forecast_points", len(forecast.Values),
		"collect_ms", collectDuration.Milliseconds(),
		"predict_ms", predictDuration.Milliseconds(),
		"capacity_ms", capacityDuration.Milliseconds(),
		"total_ms", totalDuration.Milliseconds(),
	)

	return nil
}

func (wf *WorkloadForecaster) collect(ctx context.Context) (*adapters.DataFrame, time.Duration, error) {
	start := time.Now()

	df, err := wf.adapter.Collect(ctx, int(wf.window.Seconds()))
	if err != nil {
		return nil, 0, err
	}

	duration := time.Since(start)

	if wf.metrics != nil {
		wf.metrics.RecordCollect(duration.Seconds())
	}

	wf.logger.Info("collected metrics",
		"adapter", wf.adapter.Name(),
		"rows", len(df.Rows),
		"window_seconds", int(wf.window.Seconds()),
		"duration_ms", duration.Milliseconds(),
	)

	return df, duration, nil
}

func (wf *WorkloadForecaster) buildFeatures(df *adapters.DataFrame) (models.FeatureFrame, error) {
	featureFrame, err := wf.builder.BuildFeatures(*df)
	if err != nil {
		return models.FeatureFrame{}, err
	}

	wf.logger.Debug("built features", "rows", len(featureFrame.Rows))
	return featureFrame, nil
}

func (wf *WorkloadForecaster) predict(ctx context.Context, features models.FeatureFrame) (models.Forecast, time.Duration, error) {
	start := time.Now()

	forecast, err := wf.model.Predict(ctx, features)
	if err != nil {
		return models.Forecast{}, 0, err
	}

	duration := time.Since(start)

	if wf.metrics != nil {
		wf.metrics.RecordPredict(duration.Seconds())
	}

	wf.logger.Debug("predicted forecast",
		"model", wf.model.Name(),
		"values", len(forecast.Values),
		"duration_ms", duration.Milliseconds(),
	)

	return forecast, duration, nil
}

func (wf *WorkloadForecaster) calculateReplicas(values []float64, quantiles map[float64][]float64) ([]int, time.Duration) {
	start := time.Now()

	desiredReplicas := capacity.ToReplicas(
		wf.currentReplicas,
		values,
		int(wf.step.Seconds()),
		*wf.policy,
		quantiles,
	)

	if len(desiredReplicas) > 0 {
		wf.currentReplicas = desiredReplicas[0]
	}

	duration := time.Since(start)

	if wf.metrics != nil {
		wf.metrics.RecordCapacity(duration.Seconds())
	}

	wf.logger.Debug("calculated replicas",
		"current", wf.currentReplicas,
		"duration_ms", duration.Milliseconds(),
	)

	return desiredReplicas, duration
}

func (wf *WorkloadForecaster) storeSnapshot(ctx context.Context, forecast models.Forecast, desiredReplicas []int) error {
	snapshot := storage.Snapshot{
		Workload:        wf.name,
		Metric:          forecast.Metric,
		GeneratedAt:     time.Now(),
		StepSeconds:     int(wf.step.Seconds()),
		HorizonSeconds:  int(wf.horizon.Seconds()),
		Values:          forecast.Values,
		DesiredReplicas: desiredReplicas,
		Quantiles:       forecast.Quantiles,
	}

	if err := wf.store.Put(ctx, snapshot); err != nil {
		return err
	}

	wf.logger.Debug("stored snapshot")
	return nil
}
