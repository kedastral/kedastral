package storage

import (
	"context"
	"time"
)

type Snapshot struct {
	Workload        string
	Metric          string
	GeneratedAt     time.Time
	StepSeconds     int
	HorizonSeconds  int
	Values          []float64
	DesiredReplicas []int

	// Quantiles contains optional quantile predictions for uncertainty estimation.
	// Keys are quantile levels (e.g., 0.5, 0.75, 0.9, 0.95).
	// Each value is a slice of predictions matching the length of Values.
	// If nil or empty, quantile forecasts were not available.
	Quantiles map[float64][]float64 `json:"quantiles,omitempty"`
}

type Store interface {
	Put(ctx context.Context, snapshot Snapshot) error
	GetLatest(ctx context.Context, workload string) (Snapshot, bool, error)
}
