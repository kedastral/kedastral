package main

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/HatiCode/kedastral/cmd/forecaster/metrics"
	"github.com/HatiCode/kedastral/pkg/adapters"
	"github.com/HatiCode/kedastral/pkg/capacity"
	"github.com/HatiCode/kedastral/pkg/features"
	"github.com/HatiCode/kedastral/pkg/models"
	"github.com/HatiCode/kedastral/pkg/storage"
)

func testForecaster(name string, store storage.Store) *WorkloadForecaster {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewWorkloadForecaster(
		name,
		&adapters.PrometheusAdapter{ServerURL: "http://localhost:9090", Query: "x", StepSeconds: 60},
		models.NewBaselineModel(name, 60, 1800),
		features.NewBuilder(),
		store,
		&capacity.Policy{TargetPerPod: 100, MinReplicas: 1, MaxReplicas: 10},
		30*time.Minute, // horizon
		1*time.Minute,  // step
		30*time.Minute, // window
		1*time.Hour,    // interval (long so ticks don't run during the test)
		logger,
		metrics.GetOrCreate("dynamic-"+name),
	)
}

func TestMultiForecaster_UpsertRemove(t *testing.T) {
	store := storage.NewMemoryStore()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mf := NewMultiForecaster(nil, store, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mf.Start(ctx)

	mf.Upsert(testForecaster("api", store))
	if mf.Len() != 1 {
		t.Fatalf("Len after upsert = %d, want 1", mf.Len())
	}

	// Upsert with the same name replaces rather than adds.
	mf.Upsert(testForecaster("api", store))
	if mf.Len() != 1 {
		t.Fatalf("Len after replacing upsert = %d, want 1", mf.Len())
	}

	mf.Upsert(testForecaster("worker", store))
	if mf.Len() != 2 {
		t.Fatalf("Len after second upsert = %d, want 2", mf.Len())
	}

	mf.Remove("api")
	if mf.Len() != 1 {
		t.Fatalf("Len after remove = %d, want 1", mf.Len())
	}

	// Removing an unknown workload is a no-op.
	mf.Remove("does-not-exist")
	if mf.Len() != 1 {
		t.Fatalf("Len after removing unknown = %d, want 1", mf.Len())
	}
}

func TestMultiForecaster_RemoveDeletesSnapshot(t *testing.T) {
	store := storage.NewMemoryStore()
	if err := store.Put(context.Background(), storage.Snapshot{Workload: "api"}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mf := NewMultiForecaster(nil, store, logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mf.Start(ctx)

	mf.Upsert(testForecaster("api", store))
	mf.Remove("api")

	if _, found, _ := store.GetLatest(context.Background(), "api"); found {
		t.Error("snapshot should be deleted from store after Remove")
	}
}

func TestMultiForecaster_ConcurrentUpsertRemove(t *testing.T) {
	store := storage.NewMemoryStore()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mf := NewMultiForecaster(nil, store, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mf.Start(ctx)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mf.Upsert(testForecaster("api", store))
			mf.Remove("api")
		}()
	}
	wg.Wait()
}
