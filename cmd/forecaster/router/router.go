// Package router configures HTTP routes for the forecaster's HTTP API.
//
// The forecaster exposes an HTTP server on port 8081 (configurable) that provides
// forecast snapshot retrieval, health checks, and Prometheus metrics. This package
// sets up the routes for that HTTP server.
//
// Routes configured:
//   - GET /forecast/current?workload=<name> - Retrieve latest forecast snapshot
//   - GET /healthz - Health check endpoint (returns 200 OK)
//   - GET /metrics - Prometheus metrics endpoint
//
// The /forecast/current endpoint returns forecast snapshots in JSON format as
// specified in SPEC.md §3.1, including forecast values, desired replica counts,
// and metadata (generated timestamp, step size, horizon). Snapshots older than
// the stale threshold include an X-Kedastral-Stale header.
package router

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/HatiCode/kedastral/pkg/httpx"
	"github.com/HatiCode/kedastral/pkg/storage"
)

var workloadNameRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9_-]{0,251}[a-zA-Z0-9])?$`)

// SetupRoutes configures HTTP endpoints for the forecaster.
func SetupRoutes(store storage.Store, staleAfter time.Duration, logger *slog.Logger) *http.ServeMux {
	mux := http.NewServeMux()

	// Health check endpoint
	mux.Handle("/healthz", httpx.HealthHandler())

	// Forecast snapshot endpoint
	mux.HandleFunc("/forecast/current", handleGetSnapshot(store, staleAfter, logger))

	// Prometheus metrics endpoint
	mux.Handle("/metrics", promhttp.Handler())

	return mux
}

// handleGetSnapshot returns a handler for GET /forecast/current?workload=<name>.
func handleGetSnapshot(store storage.Store, staleAfter time.Duration, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workload := r.URL.Query().Get("workload")
		if workload == "" {
			httpx.WriteErrorMessage(w, http.StatusBadRequest, "workload parameter required")
			return
		}

		if !workloadNameRegex.MatchString(workload) {
			httpx.WriteErrorMessage(w, http.StatusBadRequest, "invalid workload name format")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		snapshot, found, err := store.GetLatest(ctx, workload)
		if err != nil {
			logger.Error("failed to get snapshot", "workload", workload, "error", err)
			httpx.WriteErrorMessage(w, http.StatusInternalServerError, "internal server error")
			return
		}

		if !found {
			httpx.WriteErrorMessage(w, http.StatusNotFound, fmt.Sprintf("snapshot not found for workload %q", workload))
			return
		}

		if time.Since(snapshot.GeneratedAt) > staleAfter {
			w.Header().Set("X-Kedastral-Stale", "true")
		}

		resp := map[string]any{
			"workload":        snapshot.Workload,
			"metric":          snapshot.Metric,
			"generatedAt":     snapshot.GeneratedAt.Format(time.RFC3339),
			"stepSeconds":     snapshot.StepSeconds,
			"horizonSeconds":  snapshot.HorizonSeconds,
			"values":          snapshot.Values,
			"desiredReplicas": snapshot.DesiredReplicas,
		}

		if err := httpx.WriteJSON(w, http.StatusOK, resp); err != nil {
			logger.Error("failed to write JSON response", "error", err)
		}
	}
}
