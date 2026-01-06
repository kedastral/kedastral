// Package adapters provides Kedastral data source connectors that retrieve
// metrics or contextual signals from external systems and normalize them
// into a common DataFrame structure.
//
// Each adapter implements the Adapter interface and can be plugged into
// the Kedastral Forecast Engine. Available adapters include:
//   - PrometheusAdapter      — fetches metrics via the Prometheus HTTP API
//   - HTTPAdapter            — generic adapter for any REST API with JSON responses
//   - VictoriaMetricsAdapter — fetches metrics via VictoriaMetrics Prometheus-compatible API
//
// Future adapters (planned):
//   - ScheduleAdapter     — provides upcoming time-based events (e.g. matches)
//   - KafkaAdapter        — reads lag, queue depth, or message rate
//   - VictoriaLogsAdapter — log-based metrics from VictoriaLogs
//
// Adapters are intentionally lightweight. They focus on pulling raw data,
// shaping it into [DataFrame] objects, and leaving all feature building and
// forecasting logic to Kedastral’s upper layers.
package adapters

import (
	"context"
	"time"
)

// Row represents a single time-series or tabular observation.
// Example: {"ts": "2025-10-25T17:00:00Z", "value": 312.4, "playerA": "Nadal"}
type Row map[string]any

// DataFrame is a lightweight structure for tabular data returned by adapters.
// Each adapter collects data over a time window and returns it in this format.
type DataFrame struct {
	Rows []Row
}

// Adapter is the interface that all Kedastral adapters must implement.
//
// Adapters are responsible for fetching raw data from an external system
// (Prometheus, Kafka, HTTP API, etc.), shaping it into a DataFrame, and
// returning it for feature building and forecasting.
//
// The Collect() call is synchronous and should respect context cancellation
// and deadlines.
type Adapter interface {
	// Collect fetches metrics or events for the last windowSeconds and returns them
	// as a DataFrame. It must handle transient errors gracefully and never panic.
	Collect(ctx context.Context, windowSeconds int) (*DataFrame, error)

	// Name returns a short, unique identifier for the adapter.
	// Example: "prometheus", "schedule", "http".
	Name() string
}

// Optional: helper to align timestamps to a consistent step duration.
func AlignTimestamp(ts time.Time, stepSec int) time.Time {
	return ts.Truncate(time.Duration(stepSec) * time.Second)
}
