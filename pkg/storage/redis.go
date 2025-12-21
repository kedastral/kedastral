// Package storage provides forecast snapshot storage implementations.
package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStore implements the Store interface using Redis as a backend.
// It enables multi-instance forecaster deployments by providing shared
// storage for forecast snapshots with configurable TTL-based expiration.
type RedisStore struct {
	client *redis.Client
	ttl    time.Duration
	mu     sync.RWMutex
}

// NewRedisStore creates a new Redis-backed store.
//
// Parameters:
//   - addr: Redis server address (e.g., "localhost:6379")
//   - password: Redis password (empty string for no auth)
//   - db: Redis database number (typically 0)
//   - ttl: Snapshot expiration duration (0 uses default of 30 minutes)
//
// Returns an error if the connection to Redis fails or if parameters are invalid.
func NewRedisStore(addr, password string, db int, ttl time.Duration) (*RedisStore, error) {
	if addr == "" {
		return nil, errors.New("redis address cannot be empty")
	}
	if db < 0 {
		return nil, errors.New("redis database number must be >= 0")
	}

	if ttl == 0 {
		ttl = 30 * time.Minute
	}

	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           db,
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis at %s: %w", addr, err)
	}

	return &RedisStore{
		client: client,
		ttl:    ttl,
	}, nil
}

// Put stores a forecast snapshot in Redis with TTL-based expiration.
// The key format is "kedastral:snapshot:{workload}".
func (r *RedisStore) Put(ctx context.Context, s Snapshot) error {
	if s.Workload == "" {
		return errors.New("workload name required")
	}

	for _, c := range s.Workload {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_') {
			return fmt.Errorf("invalid workload name %q: only alphanumeric, hyphens, and underscores allowed", s.Workload)
		}
	}

	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	key := fmt.Sprintf("kedastral:snapshot:%s", s.Workload)

	if err := r.client.Set(ctx, key, data, r.ttl).Err(); err != nil {
		return fmt.Errorf("failed to store snapshot in redis: %w", err)
	}

	return nil
}

// GetLatest retrieves the latest forecast snapshot for a workload.
//
// Returns:
//   - snapshot: The forecast snapshot (zero value if not found)
//   - found: true if snapshot exists, false if not found
//   - error: non-nil if an error occurred (excluding "not found")
func (r *RedisStore) GetLatest(ctx context.Context, workload string) (Snapshot, bool, error) {
	if workload == "" {
		return Snapshot{}, false, errors.New("workload name required")
	}

	key := fmt.Sprintf("kedastral:snapshot:%s", workload)

	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return Snapshot{}, false, nil
		}
		return Snapshot{}, false, fmt.Errorf("failed to get snapshot from redis: %w", err)
	}

	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return Snapshot{}, false, fmt.Errorf("failed to unmarshal snapshot: %w", err)
	}

	return snapshot, true, nil
}

// Close closes the Redis client connection.
// It is safe to call multiple times (idempotent).
func (r *RedisStore) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.client == nil {
		return nil
	}

	err := r.client.Close()
	r.client = nil
	if err != nil && err.Error() == "redis: client is closed" {
		return nil
	}

	return err
}

// Ping checks the Redis connection health.
// Returns an error if the connection is unavailable.
func (r *RedisStore) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}
