// Command scaler implements the KEDA External Scaler for Kedastral.
//
// The scaler acts as a gRPC server that implements the KEDA External Scaler protocol,
// fetching forecast data from the Kedastral forecaster and returning predicted replica
// counts to KEDA. It supports configurable lead time to pre-scale workloads before
// anticipated load changes.
//
// Usage:
//
//	scaler -forecaster-url=http://forecaster:8081 -lead-time=5m
//
// Environment variables:
//
//	FORECASTER_URL - HTTP endpoint of the forecaster service
//	SCALER_LISTEN  - gRPC listen address (default: :50051)
//	LEAD_TIME      - Lead time for forecast selection (default: 5m)
//	LOG_LEVEL      - Logging level: debug, info, warn, error (default: info)
//	LOG_FORMAT     - Logging format: text, json (default: text)
package main

import (
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/HatiCode/kedastral/cmd/scaler/config"
	"github.com/HatiCode/kedastral/cmd/scaler/logger"
	"github.com/HatiCode/kedastral/cmd/scaler/metrics"
	"github.com/HatiCode/kedastral/cmd/scaler/router"
	pb "github.com/HatiCode/kedastral/pkg/api/externalscaler"
	"github.com/HatiCode/kedastral/pkg/httpx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

// version is set via ldflags at build time
var version = "dev"

func main() {
	cfg := config.ParseFlags()
	log := logger.New(cfg)
	m := metrics.New()

	log.Info("starting kedastral scaler",
		"version", version,
		"listen", cfg.Listen,
		"forecaster_url", cfg.ForecasterURL,
		"lead_time", cfg.LeadTime,
		"tls_enabled", cfg.TLS.Enabled,
	)

	if err := cfg.TLS.Validate(); err != nil {
		log.Error("invalid TLS configuration", "error", err)
		os.Exit(1)
	}

	scaler, err := New(cfg.ForecasterURL, cfg.LeadTime, cfg.TLS, log, m)
	if err != nil {
		log.Error("failed to create scaler", "error", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer()

	pb.RegisterExternalScalerServer(grpcServer, scaler)

	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	reflection.Register(grpcServer)

	lis, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		log.Error("failed to listen", "error", err)
		os.Exit(1)
	}

	go func() {
		log.Info("grpc server listening", "address", cfg.Listen)
		if err := grpcServer.Serve(lis); err != nil {
			log.Error("grpc server failed", "error", err)
			os.Exit(1)
		}
	}()

	httpMux := router.SetupRoutes(log)
	httpServer := httpx.NewServer(":8082", httpMux, log)

	go func() {
		if err := httpServer.Start(); err != nil {
			log.Error("http server failed", "error", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	sig := <-sigChan
	log.Info("received shutdown signal", "signal", sig)

	log.Info("shutting down grpc server")
	grpcServer.GracefulStop()

	log.Info("shutting down http server")
	if err := httpServer.Stop(10 * time.Second); err != nil {
		log.Error("http server shutdown error", "error", err)
	}

	log.Info("shutdown complete")
}
