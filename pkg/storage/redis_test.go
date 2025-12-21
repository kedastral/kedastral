//go:build integration

package storage

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/redis"
)

// setupRedisContainer starts a Redis container for testing
func setupRedisContainer(t *testing.T) (*redis.RedisContainer, string) {
	t.Helper()

	ctx := context.Background()

	redisContainer, err := redis.Run(ctx,
		"redis:7-alpine",
		redis.WithSnapshotting(10, 1),
		redis.WithLogLevel(redis.LogLevelVerbose),
	)
	if err != nil {
		t.Fatalf("failed to start redis container: %v", err)
	}

	// Get the connection string and strip redis:// prefix
	endpoint, err := redisContainer.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("failed to get redis endpoint: %v", err)
	}

	// Strip "redis://" prefix if present
	addr := endpoint
	if len(endpoint) > 8 && endpoint[:8] == "redis://" {
		addr = endpoint[8:]
	}

	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(redisContainer); err != nil {
			t.Logf("failed to terminate container: %v", err)
		}
	})

	return redisContainer, addr
}

func TestRedisStore_NewRedisStore_Success(t *testing.T) {
	_, addr := setupRedisContainer(t)

	store, err := NewRedisStore(addr, "", 0, 1*time.Minute)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	defer store.Close()

	// Verify Ping succeeds
	ctx := context.Background()
	if err := store.Ping(ctx); err != nil {
		t.Errorf("Ping failed: %v", err)
	}
}

func TestRedisStore_NewRedisStore_InvalidAddr(t *testing.T) {
	_, err := NewRedisStore("invalid:99999", "", 0, 1*time.Minute)
	if err == nil {
		t.Fatal("expected error for invalid address, got nil")
	}
}

func TestRedisStore_NewRedisStore_EmptyAddr(t *testing.T) {
	_, err := NewRedisStore("", "", 0, 1*time.Minute)
	if err == nil {
		t.Fatal("expected error for empty address, got nil")
	}
	if err.Error() != "redis address cannot be empty" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRedisStore_NewRedisStore_InvalidDB(t *testing.T) {
	_, err := NewRedisStore("localhost:6379", "", -1, 1*time.Minute)
	if err == nil {
		t.Fatal("expected error for negative db number, got nil")
	}
	if err.Error() != "redis database number must be >= 0" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRedisStore_Put_Success(t *testing.T) {
	_, addr := setupRedisContainer(t)

	store, err := NewRedisStore(addr, "", 0, 1*time.Minute)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	snapshot := Snapshot{
		Workload:        "test-api",
		Metric:          "http_rps",
		GeneratedAt:     time.Now(),
		StepSeconds:     60,
		HorizonSeconds:  1800,
		Values:          []float64{100.0, 105.0, 110.0},
		DesiredReplicas: []int{5, 5, 6},
	}

	if err := store.Put(context.Background(), snapshot); err != nil {
		t.Errorf("Put failed: %v", err)
	}

	// Verify key exists in Redis
	ctx := context.Background()
	exists, err := store.client.Exists(ctx, "kedastral:snapshot:test-api").Result()
	if err != nil {
		t.Fatalf("failed to check key existence: %v", err)
	}
	if exists != 1 {
		t.Error("expected key to exist in Redis")
	}
}

func TestRedisStore_Put_EmptyWorkload(t *testing.T) {
	_, addr := setupRedisContainer(t)

	store, err := NewRedisStore(addr, "", 0, 1*time.Minute)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	snapshot := Snapshot{
		Workload: "",
		Metric:   "http_rps",
	}

	err = store.Put(context.Background(), snapshot)
	if err == nil {
		t.Fatal("expected error for empty workload, got nil")
	}
	if err.Error() != "workload name required" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRedisStore_Put_InvalidWorkloadName(t *testing.T) {
	_, addr := setupRedisContainer(t)

	store, err := NewRedisStore(addr, "", 0, 1*time.Minute)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	snapshot := Snapshot{
		Workload: "invalid/workload",
		Metric:   "http_rps",
	}

	err = store.Put(context.Background(), snapshot)
	if err == nil {
		t.Fatal("expected error for invalid workload name, got nil")
	}
}

func TestRedisStore_GetLatest_Success(t *testing.T) {
	_, addr := setupRedisContainer(t)

	store, err := NewRedisStore(addr, "", 0, 1*time.Minute)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Put a snapshot
	originalSnapshot := Snapshot{
		Workload:        "test-api",
		Metric:          "http_rps",
		GeneratedAt:     time.Now().Truncate(time.Second), // Truncate for comparison
		StepSeconds:     60,
		HorizonSeconds:  1800,
		Values:          []float64{100.0, 105.0, 110.0},
		DesiredReplicas: []int{5, 5, 6},
	}

	if err := store.Put(context.Background(), originalSnapshot); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Get it back
	snapshot, found, err := store.GetLatest(context.Background(), "test-api")
	if err != nil {
		t.Fatalf("GetLatest failed: %v", err)
	}
	if !found {
		t.Fatal("expected snapshot to be found")
	}

	// Verify snapshot matches
	if snapshot.Workload != originalSnapshot.Workload {
		t.Errorf("workload mismatch: got %s, want %s", snapshot.Workload, originalSnapshot.Workload)
	}
	if snapshot.Metric != originalSnapshot.Metric {
		t.Errorf("metric mismatch: got %s, want %s", snapshot.Metric, originalSnapshot.Metric)
	}
	if len(snapshot.Values) != len(originalSnapshot.Values) {
		t.Errorf("values length mismatch: got %d, want %d", len(snapshot.Values), len(originalSnapshot.Values))
	}
	if len(snapshot.DesiredReplicas) != len(originalSnapshot.DesiredReplicas) {
		t.Errorf("replicas length mismatch: got %d, want %d", len(snapshot.DesiredReplicas), len(originalSnapshot.DesiredReplicas))
	}
}

func TestRedisStore_GetLatest_NotFound(t *testing.T) {
	_, addr := setupRedisContainer(t)

	store, err := NewRedisStore(addr, "", 0, 1*time.Minute)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	snapshot, found, err := store.GetLatest(context.Background(), "nonexistent")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if found {
		t.Error("expected snapshot not to be found")
	}
	if snapshot.Workload != "" {
		t.Error("expected zero-value snapshot")
	}
}

func TestRedisStore_GetLatest_EmptyWorkload(t *testing.T) {
	_, addr := setupRedisContainer(t)

	store, err := NewRedisStore(addr, "", 0, 1*time.Minute)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	_, found, err := store.GetLatest(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty workload, got nil")
	}
	if found {
		t.Error("expected found=false")
	}
	if err.Error() != "workload name required" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRedisStore_TTL_Expiration(t *testing.T) {
	_, addr := setupRedisContainer(t)

	// Create store with very short TTL
	store, err := NewRedisStore(addr, "", 0, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	snapshot := Snapshot{
		Workload:        "test-api",
		Metric:          "http_rps",
		GeneratedAt:     time.Now(),
		StepSeconds:     60,
		HorizonSeconds:  1800,
		Values:          []float64{100.0},
		DesiredReplicas: []int{5},
	}

	if err := store.Put(context.Background(), snapshot); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Verify it exists immediately
	_, found, err := store.GetLatest(context.Background(), "test-api")
	if err != nil {
		t.Fatalf("GetLatest failed: %v", err)
	}
	if !found {
		t.Fatal("expected snapshot to be found immediately after Put")
	}

	// Wait for expiration
	time.Sleep(3 * time.Second)

	// Verify it's expired
	_, found, err = store.GetLatest(context.Background(), "test-api")
	if err != nil {
		t.Fatalf("GetLatest failed: %v", err)
	}
	if found {
		t.Error("expected snapshot to be expired")
	}
}

func TestRedisStore_Concurrency_MultiplePuts(t *testing.T) {
	_, addr := setupRedisContainer(t)

	store, err := NewRedisStore(addr, "", 0, 1*time.Minute)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Launch 10 goroutines, each putting 10 snapshots
	var wg sync.WaitGroup
	numGoroutines := 10
	numPutsPerGoroutine := 10

	for i := range numGoroutines {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := range numPutsPerGoroutine {
				snapshot := Snapshot{
					Workload:        fmt.Sprintf("workload-%d-%d", goroutineID, j),
					Metric:          "http_rps",
					GeneratedAt:     time.Now(),
					StepSeconds:     60,
					HorizonSeconds:  1800,
					Values:          []float64{float64(j)},
					DesiredReplicas: []int{j},
				}

				if err := store.Put(context.Background(), snapshot); err != nil {
					t.Errorf("Put failed in goroutine %d: %v", goroutineID, err)
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify all snapshots were stored
	for i := range numGoroutines {
		for j := range numPutsPerGoroutine {
			workload := fmt.Sprintf("workload-%d-%d", i, j)
			_, found, err := store.GetLatest(context.Background(), workload)
			if err != nil {
				t.Errorf("GetLatest failed for %s: %v", workload, err)
			}
			if !found {
				t.Errorf("snapshot not found for %s", workload)
			}
		}
	}
}

func TestRedisStore_Concurrency_ReadWrite(t *testing.T) {
	_, addr := setupRedisContainer(t)

	store, err := NewRedisStore(addr, "", 0, 1*time.Minute)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Pre-populate with some snapshots
	for i := range 5 {
		snapshot := Snapshot{
			Workload:        fmt.Sprintf("workload-%d", i),
			Metric:          "http_rps",
			GeneratedAt:     time.Now(),
			StepSeconds:     60,
			HorizonSeconds:  1800,
			Values:          []float64{float64(i)},
			DesiredReplicas: []int{i},
		}
		if err := store.Put(context.Background(), snapshot); err != nil {
			t.Fatalf("initial Put failed: %v", err)
		}
	}

	// Launch 5 writers and 5 readers concurrently
	var wg sync.WaitGroup
	done := make(chan struct{})

	// Writers
	for i := range 5 {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()

			for {
				select {
				case <-done:
					return
				default:
					snapshot := Snapshot{
						Workload:        fmt.Sprintf("workload-%d", writerID),
						Metric:          "http_rps",
						GeneratedAt:     time.Now(),
						StepSeconds:     60,
						HorizonSeconds:  1800,
						Values:          []float64{float64(writerID)},
						DesiredReplicas: []int{writerID},
					}
					if err := store.Put(context.Background(), snapshot); err != nil {
						t.Errorf("Put failed in writer %d: %v", writerID, err)
					}
					time.Sleep(10 * time.Millisecond)
				}
			}
		}(i)
	}

	// Readers
	for i := range 5 {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()

			for {
				select {
				case <-done:
					return
				default:
					workload := fmt.Sprintf("workload-%d", readerID%5)
					_, _, err := store.GetLatest(context.Background(), workload)
					if err != nil {
						t.Errorf("GetLatest failed in reader %d: %v", readerID, err)
					}
					time.Sleep(10 * time.Millisecond)
				}
			}
		}(i)
	}

	// Run for 2 seconds
	time.Sleep(2 * time.Second)
	close(done)
	wg.Wait()
}

func TestRedisStore_Serialization_RoundTrip(t *testing.T) {
	_, addr := setupRedisContainer(t)

	store, err := NewRedisStore(addr, "", 0, 1*time.Minute)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Create snapshot with all fields populated
	original := Snapshot{
		Workload:        "complex-workload",
		Metric:          "custom_metric",
		GeneratedAt:     time.Now().Truncate(time.Second),
		StepSeconds:     120,
		HorizonSeconds:  3600,
		Values:          []float64{1.1, 2.2, 3.3, 4.4, 5.5},
		DesiredReplicas: []int{1, 2, 3, 4, 5},
	}

	if err := store.Put(context.Background(), original); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	retrieved, found, err := store.GetLatest(context.Background(), "complex-workload")
	if err != nil {
		t.Fatalf("GetLatest failed: %v", err)
	}
	if !found {
		t.Fatal("expected snapshot to be found")
	}

	// Verify exact equality
	if retrieved.Workload != original.Workload {
		t.Errorf("workload mismatch: got %s, want %s", retrieved.Workload, original.Workload)
	}
	if retrieved.Metric != original.Metric {
		t.Errorf("metric mismatch: got %s, want %s", retrieved.Metric, original.Metric)
	}
	if retrieved.StepSeconds != original.StepSeconds {
		t.Errorf("step mismatch: got %d, want %d", retrieved.StepSeconds, original.StepSeconds)
	}
	if retrieved.HorizonSeconds != original.HorizonSeconds {
		t.Errorf("horizon mismatch: got %d, want %d", retrieved.HorizonSeconds, original.HorizonSeconds)
	}

	// Verify slices
	if len(retrieved.Values) != len(original.Values) {
		t.Fatalf("values length mismatch: got %d, want %d", len(retrieved.Values), len(original.Values))
	}
	for i := range original.Values {
		if retrieved.Values[i] != original.Values[i] {
			t.Errorf("values[%d] mismatch: got %f, want %f", i, retrieved.Values[i], original.Values[i])
		}
	}

	if len(retrieved.DesiredReplicas) != len(original.DesiredReplicas) {
		t.Fatalf("replicas length mismatch: got %d, want %d", len(retrieved.DesiredReplicas), len(original.DesiredReplicas))
	}
	for i := range original.DesiredReplicas {
		if retrieved.DesiredReplicas[i] != original.DesiredReplicas[i] {
			t.Errorf("replicas[%d] mismatch: got %d, want %d", i, retrieved.DesiredReplicas[i], original.DesiredReplicas[i])
		}
	}
}

func TestRedisStore_Close_Idempotent(t *testing.T) {
	_, addr := setupRedisContainer(t)

	store, err := NewRedisStore(addr, "", 0, 1*time.Minute)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	// Call Close multiple times
	if err := store.Close(); err != nil {
		t.Errorf("first Close failed: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Errorf("second Close failed: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Errorf("third Close failed: %v", err)
	}
}
