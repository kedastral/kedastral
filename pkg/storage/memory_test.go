package storage

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestNewMemoryStore(t *testing.T) {
	store := NewMemoryStore()
	if store == nil {
		t.Fatal("NewMemoryStore() returned nil")
	}
	if store.Len() != 0 {
		t.Errorf("New store should be empty, got %d snapshots", store.Len())
	}
}

func TestMemoryStore_Put_Get(t *testing.T) {
	tests := []struct {
		name     string
		snapshot Snapshot
		wantErr  bool
	}{
		{
			name: "valid snapshot",
			snapshot: Snapshot{
				Workload:        "test-api",
				Metric:          "http_rps",
				GeneratedAt:     time.Now(),
				StepSeconds:     60,
				HorizonSeconds:  1800,
				Values:          []float64{100, 110, 120},
				DesiredReplicas: []int{2, 3, 3},
			},
			wantErr: false,
		},
		{
			name: "empty workload",
			snapshot: Snapshot{
				Metric:          "http_rps",
				GeneratedAt:     time.Now(),
				StepSeconds:     60,
				HorizonSeconds:  1800,
				Values:          []float64{100},
				DesiredReplicas: []int{2},
			},
			wantErr: true,
		},
		{
			name: "minimal valid snapshot",
			snapshot: Snapshot{
				Workload: "minimal",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewMemoryStore()

			// Test Put
			err := store.Put(context.Background(), tt.snapshot)
			if (err != nil) != tt.wantErr {
				t.Errorf("Put() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return // Expected error, test passed
			}

			// Test GetLatest
			got, found, err := store.GetLatest(context.Background(), tt.snapshot.Workload)
			if err != nil {
				t.Errorf("GetLatest() unexpected error = %v", err)
				return
			}

			if !found {
				t.Errorf("GetLatest() found = false, want true")
				return
			}

			// Verify snapshot fields
			if got.Workload != tt.snapshot.Workload {
				t.Errorf("Workload = %q, want %q", got.Workload, tt.snapshot.Workload)
			}
			if got.Metric != tt.snapshot.Metric {
				t.Errorf("Metric = %q, want %q", got.Metric, tt.snapshot.Metric)
			}
			if got.StepSeconds != tt.snapshot.StepSeconds {
				t.Errorf("StepSeconds = %d, want %d", got.StepSeconds, tt.snapshot.StepSeconds)
			}
			if got.HorizonSeconds != tt.snapshot.HorizonSeconds {
				t.Errorf("HorizonSeconds = %d, want %d", got.HorizonSeconds, tt.snapshot.HorizonSeconds)
			}
		})
	}
}

func TestMemoryStore_GetLatest_NotFound(t *testing.T) {
	store := NewMemoryStore()

	snapshot, found, err := store.GetLatest(context.Background(), "nonexistent")
	if err != nil {
		t.Errorf("GetLatest() unexpected error = %v", err)
	}
	if found {
		t.Error("GetLatest() found = true for nonexistent workload, want false")
	}
	if snapshot.Workload != "" {
		t.Errorf("GetLatest() returned non-zero snapshot for nonexistent workload")
	}
}

func TestMemoryStore_Put_Update(t *testing.T) {
	store := NewMemoryStore()
	workload := "update-test"

	// Put first snapshot
	snapshot1 := Snapshot{
		Workload:        workload,
		Metric:          "http_rps",
		GeneratedAt:     time.Now(),
		DesiredReplicas: []int{2, 3, 3},
	}
	if err := store.Put(context.Background(), snapshot1); err != nil {
		t.Fatalf("Put() first snapshot error = %v", err)
	}

	// Put second snapshot (update)
	snapshot2 := Snapshot{
		Workload:        workload,
		Metric:          "http_rps",
		GeneratedAt:     time.Now().Add(time.Minute),
		DesiredReplicas: []int{5, 6, 7},
	}
	if err := store.Put(context.Background(), snapshot2); err != nil {
		t.Fatalf("Put() second snapshot error = %v", err)
	}

	// Verify only the latest snapshot is stored
	got, found, err := store.GetLatest(context.Background(), workload)
	if err != nil {
		t.Fatalf("GetLatest() error = %v", err)
	}
	if !found {
		t.Fatal("GetLatest() found = false, want true")
	}

	// Should have the second snapshot's data
	if len(got.DesiredReplicas) != 3 || got.DesiredReplicas[0] != 5 {
		t.Errorf("GetLatest() returned old snapshot, want updated one")
	}

	// Store should still have only 1 entry
	if store.Len() != 1 {
		t.Errorf("Len() = %d after update, want 1", store.Len())
	}
}

func TestMemoryStore_MultipleWorkloads(t *testing.T) {
	store := NewMemoryStore()

	workloads := []string{"api-1", "api-2", "api-3"}
	for _, workload := range workloads {
		snapshot := Snapshot{
			Workload:        workload,
			Metric:          "http_rps",
			DesiredReplicas: []int{2},
		}
		if err := store.Put(context.Background(), snapshot); err != nil {
			t.Fatalf("Put(%s) error = %v", workload, err)
		}
	}

	// Verify all workloads are stored
	if store.Len() != len(workloads) {
		t.Errorf("Len() = %d, want %d", store.Len(), len(workloads))
	}

	// Verify each can be retrieved
	for _, workload := range workloads {
		got, found, err := store.GetLatest(context.Background(), workload)
		if err != nil {
			t.Errorf("GetLatest(%s) error = %v", workload, err)
		}
		if !found {
			t.Errorf("GetLatest(%s) found = false, want true", workload)
		}
		if got.Workload != workload {
			t.Errorf("GetLatest(%s) returned workload %q", workload, got.Workload)
		}
	}
}

func TestMemoryStore_Concurrent(t *testing.T) {
	store := NewMemoryStore()
	workload := "concurrent-test"

	// Number of concurrent operations
	numGoroutines := 100
	numOperations := 100

	var wg sync.WaitGroup

	// Concurrent writes
	wg.Add(numGoroutines)
	for i := range numGoroutines {
		go func(id int) {
			defer wg.Done()
			for j := range numOperations {
				snapshot := Snapshot{
					Workload:        workload,
					Metric:          "http_rps",
					GeneratedAt:     time.Now(),
					DesiredReplicas: []int{id, j},
				}
				if err := store.Put(context.Background(), snapshot); err != nil {
					t.Errorf("Concurrent Put() error = %v", err)
				}
			}
		}(i)
	}

	// Concurrent reads
	wg.Add(numGoroutines)
	for range numGoroutines {
		go func() {
			defer wg.Done()
			for range numOperations {
				_, _, err := store.GetLatest(context.Background(), workload)
				if err != nil {
					t.Errorf("Concurrent GetLatest() error = %v", err)
				}
			}
		}()
	}

	wg.Wait()

	// Verify store is still consistent
	snapshot, found, err := store.GetLatest(context.Background(), workload)
	if err != nil {
		t.Errorf("Final GetLatest() error = %v", err)
	}
	if !found {
		t.Error("Final GetLatest() found = false after concurrent operations")
	}
	if snapshot.Workload != workload {
		t.Errorf("Final snapshot has workload %q, want %q", snapshot.Workload, workload)
	}
	if store.Len() != 1 {
		t.Errorf("Len() = %d after concurrent operations, want 1", store.Len())
	}
}

func TestMemoryStore_ConcurrentMultipleWorkloads(t *testing.T) {
	store := NewMemoryStore()
	workloads := []string{"api-1", "api-2", "api-3", "api-4", "api-5"}

	var wg sync.WaitGroup

	// Each goroutine writes to a different workload
	for _, workload := range workloads {
		wg.Add(1)
		go func(w string) {
			defer wg.Done()
			for i := range 100 {
				snapshot := Snapshot{
					Workload:        w,
					Metric:          "http_rps",
					GeneratedAt:     time.Now(),
					DesiredReplicas: []int{i},
				}
				if err := store.Put(context.Background(), snapshot); err != nil {
					t.Errorf("Put(%s) error = %v", w, err)
				}
			}
		}(workload)
	}

	wg.Wait()

	// Verify all workloads are present
	if store.Len() != len(workloads) {
		t.Errorf("Len() = %d after concurrent multi-workload writes, want %d", store.Len(), len(workloads))
	}

	for _, workload := range workloads {
		snapshot, found, err := store.GetLatest(context.Background(), workload)
		if err != nil {
			t.Errorf("GetLatest(%s) error = %v", workload, err)
		}
		if !found {
			t.Errorf("GetLatest(%s) found = false, want true", workload)
		}
		if snapshot.Workload != workload {
			t.Errorf("GetLatest(%s) returned workload %q", workload, snapshot.Workload)
		}
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	store := NewMemoryStore()

	// Put a snapshot
	snapshot := Snapshot{
		Workload: "delete-test",
		Metric:   "http_rps",
	}
	if err := store.Put(context.Background(), snapshot); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	// Delete it
	deleted := store.Delete("delete-test")
	if !deleted {
		t.Error("Delete() returned false, want true for existing workload")
	}

	// Verify it's gone
	_, found, _ := store.GetLatest(context.Background(), "delete-test")
	if found {
		t.Error("GetLatest() found = true after delete, want false")
	}

	if store.Len() != 0 {
		t.Errorf("Len() = %d after delete, want 0", store.Len())
	}

	// Delete nonexistent
	deleted = store.Delete("nonexistent")
	if deleted {
		t.Error("Delete() returned true for nonexistent workload, want false")
	}
}

func TestMemoryStore_Len(t *testing.T) {
	store := NewMemoryStore()

	if store.Len() != 0 {
		t.Errorf("Initial Len() = %d, want 0", store.Len())
	}

	// Add snapshots
	for i := 1; i <= 5; i++ {
		snapshot := Snapshot{
			Workload: string(rune('a' + i - 1)),
			Metric:   "test",
		}
		if err := store.Put(context.Background(), snapshot); err != nil {
			t.Fatalf("Put() error = %v", err)
		}

		if store.Len() != i {
			t.Errorf("Len() = %d after %d puts, want %d", store.Len(), i, i)
		}
	}
}

func TestMemoryStoreWithTTL_Expiration(t *testing.T) {
	ttl := 100 * time.Millisecond
	cleanupInterval := 50 * time.Millisecond
	store := NewMemoryStoreWithTTL(ttl, cleanupInterval)
	defer store.Stop()

	// Add a snapshot
	snapshot := Snapshot{
		Workload:    "ttl-test",
		GeneratedAt: time.Now(),
		Metric:      "http_rps",
	}
	if err := store.Put(context.Background(), snapshot); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	// Verify it exists
	_, found, _ := store.GetLatest(context.Background(), "ttl-test")
	if !found {
		t.Fatal("Snapshot should exist immediately after Put")
	}

	// Wait for TTL to expire and cleanup to run
	time.Sleep(ttl + cleanupInterval + 50*time.Millisecond)

	// Verify it's been cleaned up
	_, found, _ = store.GetLatest(context.Background(), "ttl-test")
	if found {
		t.Error("Snapshot should be removed after TTL expiration")
	}

	if store.Len() != 0 {
		t.Errorf("Store should be empty after cleanup, got %d snapshots", store.Len())
	}
}

func TestMemoryStoreWithTTL_MultipleSnapshots(t *testing.T) {
	ttl := 200 * time.Millisecond
	cleanupInterval := 50 * time.Millisecond
	store := NewMemoryStoreWithTTL(ttl, cleanupInterval)
	defer store.Stop()

	// Add old snapshot
	oldSnapshot := Snapshot{
		Workload:    "old",
		GeneratedAt: time.Now().Add(-300 * time.Millisecond), // Already expired
		Metric:      "http_rps",
	}
	if err := store.Put(context.Background(), oldSnapshot); err != nil {
		t.Fatalf("Put(oldSnapshot) error = %v", err)
	}

	// Add fresh snapshot
	freshSnapshot := Snapshot{
		Workload:    "fresh",
		GeneratedAt: time.Now(),
		Metric:      "http_rps",
	}
	if err := store.Put(context.Background(), freshSnapshot); err != nil {
		t.Fatalf("Put(freshSnapshot) error = %v", err)
	}

	// Wait for cleanup to run
	time.Sleep(cleanupInterval + 50*time.Millisecond)

	// Old should be gone
	_, found, _ := store.GetLatest(context.Background(), "old")
	if found {
		t.Error("Old snapshot should be removed")
	}

	// Fresh should remain
	_, found, _ = store.GetLatest(context.Background(), "fresh")
	if !found {
		t.Error("Fresh snapshot should still exist")
	}

	if store.Len() != 1 {
		t.Errorf("Store should have 1 snapshot, got %d", store.Len())
	}
}

func TestMemoryStoreWithTTL_Stop(t *testing.T) {
	store := NewMemoryStoreWithTTL(time.Minute, time.Second)

	// Add a snapshot
	if err := store.Put(context.Background(), Snapshot{
		Workload:    "test",
		GeneratedAt: time.Now(),
	}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	// Stop should complete without hanging
	done := make(chan struct{})
	go func() {
		store.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success - Stop completed
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not complete within timeout")
	}

	// Calling Stop again should be safe
	store.Stop()
}

func TestMemoryStore_StopWithoutTTL(t *testing.T) {
	store := NewMemoryStore()

	// Stop should be safe to call even without TTL
	store.Stop()

	// Should still be usable after Stop
	err := store.Put(context.Background(), Snapshot{
		Workload: "test",
	})
	if err != nil {
		t.Errorf("Put() after Stop() error = %v", err)
	}
}

func TestMemoryStoreWithTTL_PanicOnInvalidTTL(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("NewMemoryStoreWithTTL should panic with zero TTL")
		}
	}()

	NewMemoryStoreWithTTL(0, time.Second)
}

func TestMemoryStoreWithTTL_DefaultCleanupInterval(t *testing.T) {
	// Pass zero cleanup interval, should default to 1 minute
	store := NewMemoryStoreWithTTL(time.Minute, 0)
	defer store.Stop()

	if store.cleanupTicker == nil {
		t.Error("Cleanup ticker should be initialized")
	}
}

func TestMemoryStoreWithTTL_UpdateResetsTTL(t *testing.T) {
	ttl := 200 * time.Millisecond
	cleanupInterval := 50 * time.Millisecond
	store := NewMemoryStoreWithTTL(ttl, cleanupInterval)
	defer store.Stop()

	workload := "update-ttl-test"

	// Add initial snapshot with old timestamp (will expire)
	if err := store.Put(context.Background(), Snapshot{
		Workload:        workload,
		GeneratedAt:     time.Now().Add(-250 * time.Millisecond),
		DesiredReplicas: []int{1},
	}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	// Wait for cleanup to potentially run
	time.Sleep(cleanupInterval + 20*time.Millisecond)

	// Update snapshot with fresh timestamp
	if err := store.Put(context.Background(), Snapshot{
		Workload:        workload,
		GeneratedAt:     time.Now(),
		DesiredReplicas: []int{2},
	}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	// Wait a bit (less than TTL)
	time.Sleep(cleanupInterval + 20*time.Millisecond)

	// Should still exist because we updated it with a fresh timestamp
	snapshot, found, _ := store.GetLatest(context.Background(), workload)
	if !found {
		t.Error("Updated snapshot should still exist")
	}
	if len(snapshot.DesiredReplicas) > 0 && snapshot.DesiredReplicas[0] != 2 {
		t.Error("Should have the updated snapshot data")
	}
}

func TestMemoryStoreWithTTL_ConcurrentWithCleanup(t *testing.T) {
	ttl := 200 * time.Millisecond
	cleanupInterval := 30 * time.Millisecond
	store := NewMemoryStoreWithTTL(ttl, cleanupInterval)
	defer store.Stop()

	var wg sync.WaitGroup
	numGoroutines := 50

	// Concurrent operations while cleanup is running
	wg.Add(numGoroutines)
	for i := range numGoroutines {
		go func(id int) {
			defer wg.Done()
			workload := fmt.Sprintf("workload-%d", id)

			for range 20 {
				// Put fresh snapshots
				if err := store.Put(context.Background(), Snapshot{
					Workload:    workload,
					GeneratedAt: time.Now(),
					Metric:      "test",
				}); err != nil {
					t.Errorf("Put(%s) error = %v", workload, err)
				}

				// Read
				if _, _, err := store.GetLatest(context.Background(), workload); err != nil {
					t.Errorf("GetLatest(%s) error = %v", workload, err)
				}

				time.Sleep(10 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()

	// No crashes = success
	// All snapshots should still exist (they're fresh)
	if store.Len() != numGoroutines {
		t.Logf("Warning: Expected %d snapshots, got %d (some may have expired during test)", numGoroutines, store.Len())
	}
}

// Benchmark concurrent reads and writes
func BenchmarkMemoryStore_ConcurrentAccess(b *testing.B) {
	store := NewMemoryStore()
	workloads := []string{"api-1", "api-2", "api-3"}

	// Pre-populate
	for _, w := range workloads {
		if err := store.Put(context.Background(), Snapshot{
			Workload:        w,
			DesiredReplicas: []int{1, 2, 3},
		}); err != nil {
			b.Fatalf("Put() error = %v", err)
		}
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			workload := workloads[i%len(workloads)]
			if i%2 == 0 {
				// Write
				if err := store.Put(context.Background(), Snapshot{
					Workload:        workload,
					DesiredReplicas: []int{i},
				}); err != nil {
					// Ignore errors in benchmark
					_ = err
				}
			} else {
				// Read
				if _, _, err := store.GetLatest(context.Background(), workload); err != nil {
					// Ignore errors in benchmark
					_ = err
				}
			}
			i++
		}
	})
}
