// Command forecaster implements the Kedastral forecast engine.
//
// The forecaster runs continuous forecast loops for one or more workloads that:
//  1. Collect historical metrics from Prometheus
//  2. Predict future workload using forecasting models
//  3. Calculate desired replica counts using capacity planning policies
//  4. Store forecast snapshots for the scaler to consume
//  5. Expose snapshots via HTTP API at /forecast/current
//
// The forecaster serves an HTTP API on port 8081 (configurable) providing:
//   - GET /forecast/current?workload=<name> - Retrieve latest forecast snapshot
//   - GET /healthz - Health check endpoint
//   - GET /metrics - Prometheus metrics endpoint
//
// Single-workload mode (legacy, backward compatible):
//
//	forecaster \
//	  -workload=my-api \
//	  -metric=http_rps \
//	  -prom-url=http://prometheus:9090 \
//	  -prom-query='sum(rate(http_requests_total[1m]))' \
//	  -target-per-pod=100 \
//	  -min=2 -max=50
//
// Multi-workload mode (recommended):
//
//	forecaster -config-file=/etc/kedastral/workloads.yaml
//
// Environment variables:
//
//	CONFIG_FILE    - Path to multi-workload YAML config
//	WORKLOAD       - Workload name (single-workload mode)
//	METRIC         - Metric name (single-workload mode)
//	PROM_URL       - Prometheus server URL
//	PROM_QUERY     - PromQL query (single-workload mode)
//	TLS_ENABLED    - Enable TLS for HTTP server (default: false)
//	TLS_CERT_FILE  - TLS certificate file path
//	TLS_KEY_FILE   - TLS private key file path
//	TLS_CA_FILE    - TLS CA certificate file path for client verification
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
	"github.com/HatiCode/kedastral/pkg/tls"
)

var version = "dev"

func main() {
	cfg := config.ParseFlags()

	log := logger.New(cfg)
	slog.SetDefault(log)

	log.Info("starting kedastral forecaster", "version", version)

	if err := cfg.TLS.Validate(); err != nil {
		log.Error("invalid TLS configuration", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
	workloadConfigs, err := config.LoadWorkloads(ctx, cfg)
	if err != nil {
		log.Error("failed to load workloads", "error", err)
		os.Exit(1)
	}

	log.Info("loaded workloads", "count", len(workloadConfigs))

	st := store.New(cfg, log)
	if closer, ok := st.(interface{ Close() error }); ok {
		defer func() {
			if err := closer.Close(); err != nil {
				log.Error("failed to close store", "error", err)
			}
		}()
	}

	forecasters := make([]*WorkloadForecaster, 0, len(workloadConfigs))
	for _, wc := range workloadConfigs {
		adapter := &adapters.PrometheusAdapter{
			ServerURL:   wc.PromURL,
			Query:       wc.PromQuery,
			StepSeconds: int(wc.Step.Seconds()),
		}

		model := models.NewForWorkload(wc, log)

		builder := features.NewBuilder()

		quantileLevel, err := capacity.ParseQuantileLevel(wc.QuantileLevel)
		if err != nil {
			log.Error("invalid quantile level", "workload", wc.Name, "value", wc.QuantileLevel, "error", err)
			os.Exit(1)
		}
		if quantileLevel > 0 {
			log.Info("quantile-based capacity planning enabled",
				"workload", wc.Name,
				"quantile", capacity.FormatQuantileLevel(quantileLevel))
		}

		policy := &capacity.Policy{
			TargetPerPod:          wc.TargetPerPod,
			Headroom:              wc.Headroom,
			QuantileLevel:         quantileLevel,
			MinReplicas:           wc.MinReplicas,
			MaxReplicas:           wc.MaxReplicas,
			UpMaxFactorPerStep:    wc.UpMaxFactorPerStep,
			DownMaxPercentPerStep: wc.DownMaxPercentPerStep,
		}

		wf := NewWorkloadForecaster(
			wc.Name,
			adapter,
			model,
			builder,
			st,
			policy,
			wc.Horizon,
			wc.Step,
			wc.Window,
			wc.Interval,
			log,
			metrics.New(wc.Name),
		)

		forecasters = append(forecasters, wf)
		log.Info("configured workload forecaster",
			"workload", wc.Name,
			"metric", wc.Metric,
			"model", wc.Model,
			"horizon", wc.Horizon,
			"interval", wc.Interval,
		)
	}

	multiForecaster := NewMultiForecaster(forecasters, st, log)

	staleAfter := 2 * workloadConfigs[0].Interval
	mux := router.SetupRoutes(st, staleAfter, log)
	httpServer := httpx.NewServer(cfg.Listen, mux, log)

	if cfg.TLS.Enabled {
		tlsConfig, err := tls.NewServerTLSConfig(cfg.TLS.CertFile, cfg.TLS.KeyFile, cfg.TLS.CAFile)
		if err != nil {
			log.Error("failed to create TLS config", "error", err)
			os.Exit(1)
		}
		httpServer.SetTLSConfig(tlsConfig)
		log.Info("TLS configured", "min_version", "TLS1.3", "client_auth", "required")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := multiForecaster.Run(ctx); err != nil && err != context.Canceled {
			log.Error("multi-forecaster failed", "error", err)
		}
	}()

	serverErr := make(chan error, 1)
	go func() {
		if cfg.TLS.Enabled {
			serverErr <- httpServer.StartTLS(cfg.TLS.CertFile, cfg.TLS.KeyFile)
		} else {
			log.Warn("starting HTTP server without TLS - not recommended for production")
			serverErr <- httpServer.Start()
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	select {
	case sig := <-sigCh:
		log.Info("received shutdown signal", "signal", sig)
	case err := <-serverErr:
		if err != nil {
			log.Error("server failed", "error", err)
		}
	}

	log.Info("shutting down")
	cancel()

	if err := httpServer.Stop(10 * time.Second); err != nil {
		log.Error("server shutdown failed", "error", err)
		os.Exit(1)
	}

	log.Info("shutdown complete")
}
