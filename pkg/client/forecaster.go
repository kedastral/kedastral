// Package client provides HTTP clients for communicating with Kedastral services.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/HatiCode/kedastral/pkg/storage"
)

// ForecasterClient is an HTTP client for fetching forecast snapshots from the forecaster service.
// It is safe for concurrent use by multiple goroutines.
type ForecasterClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewForecasterClient creates a new client for the forecaster service.
// The baseURL should include the scheme and host (e.g., "http://localhost:8081").
// A default timeout of 5 seconds is used for HTTP requests.
func NewForecasterClient(baseURL string) *ForecasterClient {
	return &ForecasterClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// NewForecasterClientWithTimeout creates a new client with a custom timeout.
func NewForecasterClientWithTimeout(baseURL string, timeout time.Duration) *ForecasterClient {
	return &ForecasterClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// SnapshotResponse represents the JSON response from GET /forecast/current.
// This matches the structure defined in SPEC.md §3.1.
type SnapshotResponse struct {
	Workload        string    `json:"workload"`
	Metric          string    `json:"metric"`
	GeneratedAt     time.Time `json:"generatedAt"`
	StepSeconds     int       `json:"stepSeconds"`
	HorizonSeconds  int       `json:"horizonSeconds"`
	Values          []float64 `json:"values"`
	DesiredReplicas []int     `json:"desiredReplicas"`
}

// SnapshotResult contains the snapshot and metadata about staleness.
type SnapshotResult struct {
	Snapshot storage.Snapshot
	Stale    bool // true if X-Kedastral-Stale header was present
}

// GetSnapshot fetches the latest forecast snapshot for a workload.
// Returns the snapshot and whether it's marked as stale by the forecaster.
//
// The context can be used to cancel the request or set deadlines.
// If the workload is not found, returns an error.
func (c *ForecasterClient) GetSnapshot(ctx context.Context, workload string) (*SnapshotResult, error) {
	if workload == "" {
		return nil, fmt.Errorf("workload cannot be empty")
	}

	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	u.Path = "/forecast/current"
	query := u.Query()
	query.Set("workload", workload)
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("snapshot not found for workload %q", workload)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	stale := resp.Header.Get("X-Kedastral-Stale") == "true"

	var snapshotResp SnapshotResponse
	if err := json.NewDecoder(resp.Body).Decode(&snapshotResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	snapshot := storage.Snapshot{
		Workload:        snapshotResp.Workload,
		Metric:          snapshotResp.Metric,
		GeneratedAt:     snapshotResp.GeneratedAt,
		StepSeconds:     snapshotResp.StepSeconds,
		HorizonSeconds:  snapshotResp.HorizonSeconds,
		Values:          snapshotResp.Values,
		DesiredReplicas: snapshotResp.DesiredReplicas,
	}

	return &SnapshotResult{
		Snapshot: snapshot,
		Stale:    stale,
	}, nil
}

// ListWorkloads fetches the list of all workloads tracked by the forecaster.
func (c *ForecasterClient) ListWorkloads(ctx context.Context) ([]string, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	u.Path = "/workloads"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result struct {
		Workloads []string `json:"workloads"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Workloads, nil
}

// IsStale checks if a snapshot is older than the specified duration.
// This is a helper function for the scaler to determine staleness
// based on the snapshot's GeneratedAt timestamp.
func IsStale(snapshot storage.Snapshot, staleAfter time.Duration) bool {
	return time.Since(snapshot.GeneratedAt) > staleAfter
}
