package models

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// BYOMModel implements a model that delegates predictions to an external HTTP service.
// This allows integration with any forecasting model (Prophet, TensorFlow, custom models)
// as long as the service implements the BYOM HTTP contract defined in CLAUDE.md section 4.2.
type BYOMModel struct {
	endpoint string
	metric   string
	stepSec  int
	horizon  int
	client   *http.Client
}

type byomRequest struct {
	Now            string                   `json:"now"`
	HorizonSeconds int                      `json:"horizonSeconds"`
	StepSeconds    int                      `json:"stepSeconds"`
	Features       []map[string]interface{} `json:"features"`
}

type byomResponse struct {
	Metric string    `json:"metric"`
	Values []float64 `json:"values"`
}

// NewBYOMModel creates a new BYOM model that delegates to an external HTTP service.
func NewBYOMModel(endpoint, metric string, stepSec, horizon int) *BYOMModel {
	return &BYOMModel{
		endpoint: endpoint,
		metric:   metric,
		stepSec:  stepSec,
		horizon:  horizon,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				IdleConnTimeout:     90 * time.Second,
				DisableCompression:  false,
				DisableKeepAlives:   false,
				MaxIdleConnsPerHost: 2,
			},
		},
	}
}

// Name returns the model identifier.
func (m *BYOMModel) Name() string {
	return "byom"
}

// Train is a no-op for BYOM models since the external service handles training.
func (m *BYOMModel) Train(ctx context.Context, history FeatureFrame) error {
	return nil
}

// Predict generates a forecast by calling the external BYOM HTTP service.
func (m *BYOMModel) Predict(ctx context.Context, features FeatureFrame) (Forecast, error) {
	if len(features.Rows) == 0 {
		return Forecast{}, fmt.Errorf("byom: features cannot be empty")
	}

	reqFeatures := make([]map[string]interface{}, len(features.Rows))
	for i, row := range features.Rows {
		feature := make(map[string]interface{})
		for k, v := range row {
			feature[k] = v
		}
		reqFeatures[i] = feature
	}

	req := byomRequest{
		Now:            time.Now().UTC().Format(time.RFC3339),
		HorizonSeconds: m.horizon,
		StepSeconds:    m.stepSec,
		Features:       reqFeatures,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return Forecast{}, fmt.Errorf("byom: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", m.endpoint, bytes.NewReader(body))
	if err != nil {
		return Forecast{}, fmt.Errorf("byom: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(httpReq)
	if err != nil {
		return Forecast{}, fmt.Errorf("byom: http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return Forecast{}, fmt.Errorf("byom: http %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var byomResp byomResponse
	if err := json.NewDecoder(resp.Body).Decode(&byomResp); err != nil {
		return Forecast{}, fmt.Errorf("byom: decode response: %w", err)
	}

	expectedLen := m.horizon / m.stepSec
	if len(byomResp.Values) != expectedLen {
		return Forecast{}, fmt.Errorf("byom: expected %d predictions, got %d", expectedLen, len(byomResp.Values))
	}

	for i := range byomResp.Values {
		if byomResp.Values[i] < 0 {
			byomResp.Values[i] = 0
		}
	}

	return Forecast{
		Metric:    m.metric,
		Values:    byomResp.Values,
		StepSec:   m.stepSec,
		Horizon:   m.horizon,
		Quantiles: nil,
	}, nil
}
