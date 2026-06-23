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
//
// Workloads can be registered up front (legacy flag mode) or added and removed at
// runtime (operator mode) via Upsert and Remove. Each running workload has its own
// cancelable context derived from the base context passed to Start, so removing one
// workload does not affect the others.
type MultiForecaster struct {
	store  storage.Store
	logger *slog.Logger

	mu      sync.Mutex
	baseCtx context.Context
	started bool
	running map[string]*managedForecaster
	wg      sync.WaitGroup
}

// managedForecaster tracks a workload forecaster and the cancel function for its goroutine.
type managedForecaster struct {
	forecaster *WorkloadForecaster
	cancel     context.CancelFunc
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
// The initial workloads are registered but not started until Start (or Run) is called.
// Operator mode typically passes nil and registers workloads later via Upsert.
func NewMultiForecaster(workloads []*WorkloadForecaster, store storage.Store, logger *slog.Logger) *MultiForecaster {
	if logger == nil {
		logger = slog.Default()
	}

	running := make(map[string]*managedForecaster)
	for _, wf := range workloads {
		running[wf.name] = &managedForecaster{forecaster: wf}
	}

	return &MultiForecaster{
		store:   store,
		logger:  logger,
		running: running,
	}
}

// Start begins running all registered workload forecasters and returns immediately.
// Workloads registered afterwards via Upsert are launched as they arrive. The base
// context governs the lifetime of every workload goroutine.
func (mf *MultiForecaster) Start(ctx context.Context) {
	mf.mu.Lock()
	defer mf.mu.Unlock()

	mf.baseCtx = ctx
	mf.started = true
	mf.logger.Info("starting multi-workload forecaster", "workloads", len(mf.running))

	for name, managed := range mf.running {
		mf.launch(name, managed)
	}
}

// launch starts the goroutine for a managed forecaster. The caller must hold mf.mu
// and mf.started must be true.
func (mf *MultiForecaster) launch(name string, managed *managedForecaster) {
	workloadCtx, cancel := context.WithCancel(mf.baseCtx)
	managed.cancel = cancel

	mf.wg.Add(1)
	go func() {
		defer mf.wg.Done()
		if err := managed.forecaster.Run(workloadCtx); err != nil && err != context.Canceled {
			mf.logger.Error("workload forecaster error", "workload", name, "error", err)
		}
	}()
}

// Upsert registers or replaces a workload forecaster. If a forecaster with the same
// name is already running, it is canceled and replaced. Safe for concurrent use.
func (mf *MultiForecaster) Upsert(forecaster *WorkloadForecaster) {
	mf.mu.Lock()
	defer mf.mu.Unlock()

	name := forecaster.name
	if existing, ok := mf.running[name]; ok && existing.cancel != nil {
		existing.cancel()
	}

	managed := &managedForecaster{forecaster: forecaster}
	mf.running[name] = managed

	if mf.started {
		mf.launch(name, managed)
	}
}

// Remove stops and unregisters a workload forecaster and deletes its snapshot from
// the store. It is a no-op if the workload is not registered. Safe for concurrent use.
func (mf *MultiForecaster) Remove(name string) {
	mf.mu.Lock()
	managed, ok := mf.running[name]
	if ok {
		if managed.cancel != nil {
			managed.cancel()
		}
		delete(mf.running, name)
	}
	mf.mu.Unlock()

	if !ok {
		return
	}

	if deleter, ok := mf.store.(interface{ Delete(string) bool }); ok {
		deleter.Delete(name)
	}
	mf.logger.Info("removed workload forecaster", "workload", name)
}

// Run starts all registered workload forecasters and blocks until the context is
// canceled and every workload goroutine has exited. Used by legacy flag mode.
func (mf *MultiForecaster) Run(ctx context.Context) error {
	mf.mu.Lock()
	empty := len(mf.running) == 0
	mf.mu.Unlock()
	if empty {
		return fmt.Errorf("no workloads configured")
	}

	mf.Start(ctx)

	<-ctx.Done()
	mf.wg.Wait()
	mf.logger.Info("multi-workload forecaster stopped")
	return ctx.Err()
}

// GetStore returns the shared storage backend.
func (mf *MultiForecaster) GetStore() storage.Store {
	return mf.store
}

// Len returns the number of registered workload forecasters. Safe for concurrent use.
func (mf *MultiForecaster) Len() int {
	mf.mu.Lock()
	defer mf.mu.Unlock()
	return len(mf.running)
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
