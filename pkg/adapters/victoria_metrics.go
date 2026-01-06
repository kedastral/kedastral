package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"time"
)

// VictoriaMetricsAdapter fetches time-series data from VictoriaMetrics via its
// Prometheus-compatible HTTP API. It issues a /api/v1/query_range call and returns
// a *DataFrame with rows of the form:
//
//	{"ts": RFC3339 string, "value": float64}
//
// If multiple series are returned, values with the same timestamp are SUMMED.
type VictoriaMetricsAdapter struct {
	// ServerURL is the base URL to VictoriaMetrics, e.g. http://victoria-metrics:8428
	ServerURL string
	// Query is the MetricsQL/PromQL expression to evaluate.
	Query string
	// StepSeconds controls the resolution (defaults to 60s if <= 0).
	StepSeconds int
	// HTTPClient is optional; if nil a default client with timeout is used.
	HTTPClient *http.Client
}

func (v *VictoriaMetricsAdapter) Name() string { return "victoria-metrics" }

// Collect implements Adapter. It queries VictoriaMetrics for the last windowSeconds worth
// of data, at StepSeconds resolution, and returns a *DataFrame. It respects the
// provided context for cancellation and deadlines.
func (v *VictoriaMetricsAdapter) Collect(ctx context.Context, windowSeconds int) (*DataFrame, error) {
	if v.ServerURL == "" || v.Query == "" {
		return &DataFrame{}, errors.New("victoria metrics adapter: ServerURL and QueryURL are required")
	}
	step := v.StepSeconds
	if step <= 0 {
		step = 60
	}
	now := time.Now().UTC().Truncate(time.Second)
	start := now.Add(-time.Duration(windowSeconds) * time.Second)

	u, err := url.Parse(v.ServerURL)
	if err != nil {
		return &DataFrame{}, fmt.Errorf("invalid ServerURL: %w", err)
	}
	u.Path = "/api/v1/query_range"

	q := u.Query()
	q.Set("query", v.Query)
	q.Set("start", fmt.Sprintf("%d", start.Unix()))
	q.Set("end", fmt.Sprintf("%d", now.Unix()))
	q.Set("step", fmt.Sprintf("%d", step))
	u.RawQuery = q.Encode()

	cli := v.HTTPClient
	if cli == nil {
		cli = &http.Client{Timeout: 10 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return &DataFrame{}, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := cli.Do(req)
	if err != nil {
		return &DataFrame{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &DataFrame{}, fmt.Errorf("victoria-metrics: status %d", resp.StatusCode)
	}

	// VictoriaMetrics returns Prometheus-compatible responses
	var pr PrometheusRangeResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return &DataFrame{}, fmt.Errorf("decode victoria-metrics response: %w", err)
	}
	if pr.Status != "success" {
		return &DataFrame{}, fmt.Errorf("victoria-metrics status: %s", pr.Status)
	}

	rows, err := AggregateRangeResult(pr.Data.Result)
	if err != nil {
		return &DataFrame{}, err
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i]["ts"].(time.Time).Before(rows[j]["ts"].(time.Time))
	})

	for i := range rows {
		rows[i]["ts"] = rows[i]["ts"].(time.Time).UTC().Format(time.RFC3339)
	}

	return &DataFrame{Rows: rows}, nil
}
