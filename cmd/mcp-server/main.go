// Command mcp-server exposes Kedastral forecast data as MCP tools for AI assistants.
//
// It connects to the forecaster HTTP API and serves these tools:
//   - list_workloads: list all tracked workloads
//   - get_forecast: retrieve the full forecast snapshot for a workload
//   - explain_decision: human-readable explanation of the current scaling decision
//
// When the server has Kubernetes access (in-cluster or via kubeconfig), two
// operator-aware tools are also registered:
//   - list_forecast_policies: list ForecastPolicy resources and their status
//   - get_forecast_policy: full configuration and status for one ForecastPolicy
//
// Two transport modes are supported:
//   - stdio (default): spawned as a subprocess by local AI clients (e.g. Claude Desktop)
//   - sse: long-running HTTP service for cluster deployments
//
// Usage:
//
//	mcp-server -forecaster-url=http://forecaster:8081
//	mcp-server -transport=sse -listen=:8083 -base-url=http://kedastral-mcp:8083 -forecaster-url=http://forecaster:8081
//
// Environment variables:
//
//	FORECASTER_URL - HTTP endpoint of the forecaster service
//	STALE_AFTER    - Age threshold for stale snapshots (default: 5m)
//	MCP_TRANSPORT  - Transport mode: stdio or sse (default: stdio)
//	MCP_LISTEN     - Listen address for SSE transport (default: :8083)
//	MCP_BASE_URL   - Public base URL for SSE transport (required for sse)
//	LOG_LEVEL      - Logging level: debug, info, warn, error (default: info)
//	LOG_FORMAT     - Logging format: text, json (default: text)
package main

import (
	"os"

	"github.com/mark3labs/mcp-go/server"

	"github.com/HatiCode/kedastral/cmd/mcp-server/config"
	"github.com/HatiCode/kedastral/cmd/mcp-server/logger"
	"github.com/HatiCode/kedastral/pkg/client"
)

// version is set via ldflags at build time.
var version = "dev"

func main() {
	cfg := config.ParseFlags()
	log := logger.New(cfg)

	log.Info("starting kedastral mcp-server",
		"version", version,
		"transport", cfg.Transport,
		"forecaster_url", cfg.ForecasterURL,
		"stale_after", cfg.StaleAfter,
	)

	forecasterClient := client.NewForecasterClient(cfg.ForecasterURL)

	// Operator-aware tools are enabled only when the MCP server has Kubernetes access
	// (in-cluster or via kubeconfig). Without it, the forecaster-only tools still work.
	policyReader, err := newPolicyReader()
	if err != nil {
		log.Info("operator tools disabled: no kubernetes access", "error", err)
		policyReader = nil
	} else {
		log.Info("operator tools enabled (list_forecast_policies, get_forecast_policy)")
	}

	s := buildMCPServer(forecasterClient, policyReader, cfg.StaleAfter, version, log)

	switch cfg.Transport {
	case "sse":
		log.Info("starting SSE server", "listen", cfg.Listen, "base_url", cfg.BaseURL)
		sseServer := server.NewSSEServer(s, server.WithBaseURL(cfg.BaseURL))
		if err := sseServer.Start(cfg.Listen); err != nil {
			log.Error("SSE server error", "error", err)
			os.Exit(1)
		}
	default:
		if err := server.ServeStdio(s); err != nil {
			log.Error("stdio server error", "error", err)
			os.Exit(1)
		}
	}
}
