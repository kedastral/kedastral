package storage

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MemoryStore implements an in-memory store for forecast snapshots.
// It is safe for concurrent use by multiple goroutines.
//
// MemoryStore uses a simple map to store the latest snapshot per workload.
// If TTL is configured, a background goroutine automatically removes stale
// snapshots. For production deployments requiring persistence or multi-instance
// setups, consider using RedisStore instead.
type MemoryStore struct {
	mu            sync.RWMutex
	snapshots     map[string]Snapshot
	ttl           time.Duration
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}
	cleanupDone   chan struct{}
	stopped       bool
	stopMu        sync.Mutex
}

// NewMemoryStore creates a new in-memory snapshot store with no TTL.
// Snapshots will be stored indefinitely until explicitly deleted or updated.
// The store is ready to use immediately with no additional configuration.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		snapshots: make(map[string]Snapshot),
	}
}

// NewMemoryStoreWithTTL creates a new in-memory snapshot store with automatic
// TTL-based cleanup. A background goroutine runs every minute to remove
// snapshots older than the specified TTL.
//
// The cleanup goroutine must be stopped by calling Stop() when the store
// is no longer needed to prevent goroutine leaks.
//
// cleanupInterval determines how often the cleanup runs (typically 1 minute).
func NewMemoryStoreWithTTL(ttl, cleanupInterval time.Duration) *MemoryStore {
	if ttl <= 0 {
		panic("TTL must be positive")
	}
	if cleanupInterval <= 0 {
		cleanupInterval = time.Minute
	}

	store := &MemoryStore{
		snapshots:     make(map[string]Snapshot),
		ttl:           ttl,
		cleanupTicker: time.NewTicker(cleanupInterval),
		stopCleanup:   make(chan struct{}),
		cleanupDone:   make(chan struct{}),
	}

	go store.runCleanup()

	return store
}

// Stop gracefully shuts down the background cleanup goroutine.
// This method must be called when using a store created with NewMemoryStoreWithTTL
// to prevent goroutine leaks. It blocks until cleanup is complete.
//
// Calling Stop multiple times or on a store without TTL is safe and does nothing.
func (s *MemoryStore) Stop() {
	if s.cleanupTicker == nil {
		return // No cleanup goroutine running
	}

	s.stopMu.Lock()
	defer s.stopMu.Unlock()

	if s.stopped {
		return // Already stopped
	}

	close(s.stopCleanup)
	<-s.cleanupDone
	s.cleanupTicker.Stop()
	s.stopped = true
}

// runCleanup is the background goroutine that periodically removes stale snapshots.
func (s *MemoryStore) runCleanup() {
	defer close(s.cleanupDone)

	for {
		select {
		case <-s.cleanupTicker.C:
			s.cleanup()
		case <-s.stopCleanup:
			return
		}
	}
}

// cleanup removes snapshots older than the TTL.
func (s *MemoryStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ttl == 0 {
		return // No TTL configured
	}

	now := time.Now()
	for workload, snapshot := range s.snapshots {
		if now.Sub(snapshot.GeneratedAt) > s.ttl {
			delete(s.snapshots, workload)
		}
	}
}

// Put stores a snapshot for a workload, replacing any existing snapshot.
// The workload name is extracted from the snapshot's Workload field.
//
// Returns an error if the snapshot's Workload field is empty or if context is canceled.
// This operation is safe for concurrent use.
func (s *MemoryStore) Put(ctx context.Context, snapshot Snapshot) error {
	if snapshot.Workload == "" {
		return fmt.Errorf("snapshot workload cannot be empty")
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.snapshots[snapshot.Workload] = snapshot
	return nil
}

// GetLatest retrieves the most recent snapshot for a workload.
//
// Returns:
//   - snapshot: The stored snapshot (zero value if not found)
//   - found: true if a snapshot exists for this workload, false otherwise
//   - error: Context error if context is canceled, nil otherwise
//
// This operation is safe for concurrent use.
func (s *MemoryStore) GetLatest(ctx context.Context, workload string) (Snapshot, bool, error) {
	select {
	case <-ctx.Done():
		return Snapshot{}, false, ctx.Err()
	default:
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot, found := s.snapshots[workload]
	return snapshot, found, nil
}

// Len returns the number of snapshots currently stored.
// This method is primarily useful for testing and metrics.
//
// This operation is safe for concurrent use.
func (s *MemoryStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.snapshots)
}

// Delete removes a snapshot for a workload.
// This method is primarily useful for testing and cleanup.
// Returns true if a snapshot was deleted, false if none existed.
//
// This operation is safe for concurrent use.
func (s *MemoryStore) Delete(workload string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, existed := s.snapshots[workload]
	delete(s.snapshots, workload)
	return existed
}
