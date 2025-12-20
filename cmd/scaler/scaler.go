// Package main implements the core KEDA External Scaler gRPC service.
//
// This file contains the Scaler type which implements the ExternalScaler interface
// defined in the KEDA external scaler protocol. It handles four key gRPC methods:
//
//   - IsActive: Determines if the scaler should be active based on forecast freshness
//   - GetMetricSpec: Returns metric specifications for KEDA (metric name and target)
//   - GetMetrics: Returns current predicted replica counts from the forecaster
//   - StreamIsActive: Not implemented (returns error directing to use 'external' type)
//
// The scaler fetches forecast snapshots from the Kedastral forecaster via HTTP,
// selects the appropriate replica count based on configured lead time, and returns
// this value to KEDA for scaling decisions.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/HatiCode/kedastral/cmd/scaler/metrics"
	pb "github.com/HatiCode/kedastral/pkg/api/externalscaler"
	"github.com/HatiCode/kedastral/pkg/storage"
)

// Scaler implements the KEDA ExternalScaler gRPC interface
type Scaler struct {
	pb.UnimplementedExternalScalerServer
	forecasterURL string
	client        *http.Client
	logger        *slog.Logger
	leadTime      time.Duration
	metrics       *metrics.Metrics
}

// New creates a new scaler instance
func New(forecasterURL string, leadTime time.Duration, logger *slog.Logger, m *metrics.Metrics) *Scaler {
	if logger == nil {
		logger = slog.Default()
	}

	return &Scaler{
		forecasterURL: forecasterURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger:   logger,
		leadTime: leadTime,
		metrics:  m,
	}
}

// IsActive determines if the scaler is active for the given ScaledObject
func (s *Scaler) IsActive(ctx context.Context, ref *pb.ScaledObjectRef) (*pb.IsActiveResponse, error) {
	start := time.Now()
	defer func() {
		if s.metrics != nil {
			s.metrics.ObserveGRPCDuration("IsActive", time.Since(start).Seconds())
		}
	}()

	workload := s.getWorkload(ref)

	s.logger.Debug("IsActive called",
		"name", ref.Name,
		"namespace", ref.Namespace,
		"workload", workload,
	)

	snapshot, err := s.getForecast(ctx, workload)
	if err != nil {
		s.logger.Warn("failed to get forecast, marking inactive",
			"workload", workload,
			"error", err,
		)
		if s.metrics != nil {
			s.metrics.RecordGRPCRequest("IsActive", "inactive_error")
		}
		return &pb.IsActiveResponse{Result: false}, nil
	}

	age := time.Since(snapshot.GeneratedAt)
	staleThreshold := s.leadTime * 2
	if age > staleThreshold {
		s.logger.Warn("forecast is stale, marking inactive",
			"workload", workload,
			"age", age,
			"threshold", staleThreshold,
		)
		if s.metrics != nil {
			s.metrics.RecordGRPCRequest("IsActive", "inactive_stale")
		}
		return &pb.IsActiveResponse{Result: false}, nil
	}

	s.logger.Debug("scaler is active",
		"workload", workload,
		"forecast_age", age,
	)

	if s.metrics != nil {
		s.metrics.RecordGRPCRequest("IsActive", "active")
		s.metrics.SetForecastAge(age.Seconds())
	}

	return &pb.IsActiveResponse{Result: true}, nil
}

// StreamIsActive streams the active status (not implemented)
func (s *Scaler) StreamIsActive(ref *pb.ScaledObjectRef, stream pb.ExternalScaler_StreamIsActiveServer) error {
	s.logger.Debug("StreamIsActive called (not implemented)",
		"name", ref.Name,
		"namespace", ref.Namespace,
	)
	if s.metrics != nil {
		s.metrics.RecordGRPCRequest("StreamIsActive", "not_implemented")
	}
	return fmt.Errorf("StreamIsActive not implemented - use external type, not external-push")
}

// GetMetricSpec returns the metric spec for the scaler
func (s *Scaler) GetMetricSpec(ctx context.Context, ref *pb.ScaledObjectRef) (*pb.GetMetricSpecResponse, error) {
	start := time.Now()
	defer func() {
		if s.metrics != nil {
			s.metrics.ObserveGRPCDuration("GetMetricSpec", time.Since(start).Seconds())
		}
	}()

	workload := s.getWorkload(ref)
	metricName := s.getMetricName(ref, workload)

	s.logger.Debug("GetMetricSpec called",
		"name", ref.Name,
		"namespace", ref.Namespace,
		"workload", workload,
		"metricName", metricName,
	)

	if s.metrics != nil {
		s.metrics.RecordGRPCRequest("GetMetricSpec", "success")
	}

	return &pb.GetMetricSpecResponse{
		MetricSpecs: []*pb.MetricSpec{
			{
				MetricName:      metricName,
				TargetSizeFloat: 1.0,
			},
		},
	}, nil
}

// GetMetrics returns the current metric values
func (s *Scaler) GetMetrics(ctx context.Context, req *pb.GetMetricsRequest) (*pb.GetMetricsResponse, error) {
	start := time.Now()
	defer func() {
		if s.metrics != nil {
			s.metrics.ObserveGRPCDuration("GetMetrics", time.Since(start).Seconds())
		}
	}()

	workload := s.getWorkload(req.ScaledObjectRef)

	s.logger.Debug("GetMetrics called",
		"name", req.ScaledObjectRef.Name,
		"namespace", req.ScaledObjectRef.Namespace,
		"workload", workload,
		"metricName", req.MetricName,
	)

	snapshot, err := s.getForecast(ctx, workload)
	if err != nil {
		if s.metrics != nil {
			s.metrics.RecordGRPCRequest("GetMetrics", "error")
		}
		return nil, fmt.Errorf("failed to get forecast: %w", err)
	}

	desiredReplicas := s.selectReplicasAtLeadTime(snapshot)

	s.logger.Info("returning desired replicas",
		"workload", workload,
		"desired", desiredReplicas,
		"forecast_age", time.Since(snapshot.GeneratedAt),
	)

	if s.metrics != nil {
		s.metrics.RecordGRPCRequest("GetMetrics", "success")
		s.metrics.SetDesiredReplicas(desiredReplicas)
		s.metrics.SetForecastAge(time.Since(snapshot.GeneratedAt).Seconds())
	}

	return &pb.GetMetricsResponse{
		MetricValues: []*pb.MetricValue{
			{
				MetricName:       req.MetricName,
				MetricValueFloat: float64(desiredReplicas),
			},
		},
	}, nil
}

// getForecast fetches the latest forecast snapshot from the forecaster
func (s *Scaler) getForecast(ctx context.Context, workload string) (*storage.Snapshot, error) {
	start := time.Now()
	defer func() {
		if s.metrics != nil {
			s.metrics.ObserveForecastFetch(time.Since(start).Seconds())
		}
	}()

	url := fmt.Sprintf("%s/forecast/current?workload=%s", s.forecasterURL, workload)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		if s.metrics != nil {
			s.metrics.RecordForecastFetchError()
		}
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		if s.metrics != nil {
			s.metrics.RecordForecastFetchError()
		}
		return nil, fmt.Errorf("failed to fetch forecast: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if s.metrics != nil {
			s.metrics.RecordForecastFetchError()
		}
		return nil, fmt.Errorf("forecaster returned status %d", resp.StatusCode)
	}

	var snapshot storage.Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&snapshot); err != nil {
		if s.metrics != nil {
			s.metrics.RecordForecastFetchError()
		}
		return nil, fmt.Errorf("failed to decode forecast: %w", err)
	}

	return &snapshot, nil
}

// selectReplicasAtLeadTime returns the maximum replica count over the window [now...now+leadTime].
// This ensures we scale up proactively for upcoming spikes while staying scaled during current load.
func (s *Scaler) selectReplicasAtLeadTime(snapshot *storage.Snapshot) int {
	if len(snapshot.DesiredReplicas) == 0 {
		s.logger.Warn("no desired replicas in forecast, defaulting to 1")
		return 1
	}

	stepDuration := time.Duration(snapshot.StepSeconds) * time.Second
	leadSteps := int(s.leadTime / stepDuration)

	if leadSteps >= len(snapshot.DesiredReplicas) {
		leadSteps = len(snapshot.DesiredReplicas) - 1
	}
	if leadSteps < 0 {
		leadSteps = 0
	}

	maxReplicas := snapshot.DesiredReplicas[0]
	for i := 1; i <= leadSteps && i < len(snapshot.DesiredReplicas); i++ {
		if snapshot.DesiredReplicas[i] > maxReplicas {
			maxReplicas = snapshot.DesiredReplicas[i]
		}
	}

	s.logger.Debug("selected max replicas over lead time window",
		"lead_time", s.leadTime,
		"step_seconds", snapshot.StepSeconds,
		"lead_steps", leadSteps,
		"max_replicas", maxReplicas,
	)

	return maxReplicas
}

// getWorkload extracts the workload name from scaler metadata
func (s *Scaler) getWorkload(ref *pb.ScaledObjectRef) string {
	if workload := ref.ScalerMetadata["workload"]; workload != "" {
		return workload
	}
	return ref.Name
}

// getMetricName constructs the metric name
func (s *Scaler) getMetricName(ref *pb.ScaledObjectRef, workload string) string {
	if metricName := ref.ScalerMetadata["metricName"]; metricName != "" {
		return metricName
	}
	return fmt.Sprintf("kedastral-%s-desired-replicas", workload)
}
