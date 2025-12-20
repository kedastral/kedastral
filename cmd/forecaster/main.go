// Command forecaster implements the Kedastral forecast engine.
//
// The forecaster runs a continuous forecast loop that:
//  1. Collects historical metrics from Prometheus
//  2. Predicts future workload using a forecasting model
//  3. Calculates desired replica counts using capacity planning policies
//  4. Stores forecast snapshots for the scaler to consume
//  5. Exposes snapshots via HTTP API at /forecast/current
//
// The forecaster serves an HTTP API on port 8081 (configurable) providing:
//   - GET /forecast/current?workload=<name> - Retrieve latest forecast snapshot
//   - GET /healthz - Health check endpoint
//   - GET /metrics - Prometheus metrics endpoint
//
// Usage:
//
//	forecaster \
//	  -workload=my-api \
//	  -metric=http_rps \
//	  -prom-url=http://prometheus:9090 \
//	  -prom-query='sum(rate(http_requests_total[1m]))' \
//	  -target-per-pod=100 \
//	  -min=2 -max=50
//
// Environment variables:
//
//	WORKLOAD       - Workload name (required)
//	METRIC         - Metric name (required)
//	PROM_URL       - Prometheus server URL
//	PROM_QUERY     - PromQL query (required)
//	TARGET_PER_POD - Target metric value per pod
//	MIN_REPLICAS   - Minimum replica count
//	MAX_REPLICAS   - Maximum replica count
//	HEADROOM       - Headroom multiplier (default: 1.2)
//	HORIZON        - Forecast horizon duration (default: 30m)
//	STEP           - Forecast step size (default: 1m)
//	INTERVAL       - Forecast loop interval (default: 30s)
//	LOG_LEVEL      - Logging level: debug, info, warn, error (default: info)
//	LOG_FORMAT     - Logging format: text, json (default: text)
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/HatiCode/kedastral/cmd/forecaster/config"
	"github.com/HatiCode/kedastral/cmd/forecaster/logger"
	"github.com/HatiCode/kedastral/cmd/forecaster/metrics"
	"github.com/HatiCode/kedastral/cmd/forecaster/models"
	"github.com/HatiCode/kedastral/cmd/forecaster/router"
	"github.com/HatiCode/kedastral/cmd/forecaster/store"
	"github.com/HatiCode/kedastral/pkg/adapters"
	"github.com/HatiCode/kedastral/pkg/capacity"
	"github.com/HatiCode/kedastral/pkg/features"
	"github.com/HatiCode/kedastral/pkg/httpx"
)

// version is set via ldflags at build time
var version = "dev"

func main() {
	cfg := config.ParseFlags()

	logger := logger.New(cfg)
	slog.SetDefault(logger)

	logger.Info("starting kedastral forecaster",
		"version", version,
		"workload", cfg.Workload,
		"metric", cfg.Metric,
	)

	adapter := &adapters.PrometheusAdapter{
		ServerURL:   cfg.PromURL,
		Query:       cfg.PromQuery,
		StepSeconds: int(cfg.Step.Seconds()),
	}

	model := models.New(cfg, logger)

	builder := features.NewBuilder()

	store := store.New(cfg, logger)
	if closer, ok := store.(interface{ Close() error }); ok {
		defer func() {
			if err := closer.Close(); err != nil {
				logger.Error("failed to close store", "error", err)
			}
		}()
	}

	policy := &capacity.Policy{
		TargetPerPod:          cfg.TargetPerPod,
		Headroom:              cfg.Headroom,
		MinReplicas:           cfg.MinReplicas,
		MaxReplicas:           cfg.MaxReplicas,
		UpMaxFactorPerStep:    cfg.UpMaxFactorPerStep,
		DownMaxPercentPerStep: cfg.DownMaxPercentPerStep,
	}

	f := New(
		cfg.Workload,
		adapter,
		model,
		builder,
		store,
		policy,
		cfg.Horizon,
		cfg.Step,
		cfg.Window,
		logger,
		metrics.New(cfg.Workload),
	)

	staleAfter := 2 * cfg.Interval // Snapshot is stale if older than 2x the interval
	mux := router.SetupRoutes(store, staleAfter, logger)
	httpServer := httpx.NewServer(cfg.Listen, mux, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := f.Run(ctx, cfg.Interval); err != nil && err != context.Canceled {
			logger.Error("forecast loop failed", "error", err)
		}
	}()

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- httpServer.Start()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	select {
	case sig := <-sigCh:
		logger.Info("received shutdown signal", "signal", sig)
	case err := <-serverErr:
		if err != nil {
			logger.Error("server failed", "error", err)
		}
	}

	logger.Info("shutting down")
	cancel()

	if err := httpServer.Stop(10 * time.Second); err != nil {
		logger.Error("server shutdown failed", "error", err)
		os.Exit(1)
	}

	logger.Info("shutdown complete")
}
