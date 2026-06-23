// Package main implements the Kedastral MCP server, exposing forecast data
// and scaling decisions as tools consumable by AI assistants.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/HatiCode/kedastral/pkg/client"
)

func buildMCPServer(forecasterClient *client.ForecasterClient, policyReader PolicyReader, staleAfter time.Duration, version string, log *slog.Logger) *server.MCPServer {
	s := server.NewMCPServer(
		"kedastral",
		version,
		server.WithToolCapabilities(false),
	)

	s.AddTool(
		mcp.NewTool("list_workloads",
			mcp.WithDescription("List all workloads currently tracked by the Kedastral forecaster."),
		),
		handleListWorkloads(forecasterClient, log),
	)

	s.AddTool(
		mcp.NewTool("get_forecast",
			mcp.WithDescription("Get the latest forecast snapshot for a workload, including predicted metric values and desired replica counts over the forecast horizon."),
			mcp.WithString("workload",
				mcp.Required(),
				mcp.Description("Name of the workload (e.g. my-api)"),
			),
		),
		handleGetForecast(forecasterClient, staleAfter, log),
	)

	s.AddTool(
		mcp.NewTool("explain_decision",
			mcp.WithDescription("Return a human-readable explanation of the current scaling decision for a workload, including trend, peak replicas, and staleness."),
			mcp.WithString("workload",
				mcp.Required(),
				mcp.Description("Name of the workload (e.g. my-api)"),
			),
		),
		handleExplainDecision(forecasterClient, staleAfter, log),
	)

	if policyReader != nil {
		registerPolicyTools(s, policyReader, log)
	}

	return s
}

func handleListWorkloads(fc *client.ForecasterClient, log *slog.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workloads, err := fc.ListWorkloads(ctx)
		if err != nil {
			log.Error("list_workloads failed", "error", err)
			return mcp.NewToolResultError(fmt.Sprintf("failed to list workloads: %v", err)), nil
		}

		if len(workloads) == 0 {
			return mcp.NewToolResultText("No workloads are currently tracked."), nil
		}

		return mcp.NewToolResultText("Tracked workloads:\n- " + strings.Join(workloads, "\n- ")), nil
	}
}

func handleGetForecast(fc *client.ForecasterClient, staleAfter time.Duration, log *slog.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workload, err := req.RequireString("workload")
		if err != nil {
			return mcp.NewToolResultError("workload parameter is required"), nil
		}

		result, err := fc.GetSnapshot(ctx, workload)
		if err != nil {
			log.Error("get_forecast failed", "workload", workload, "error", err)
			return mcp.NewToolResultError(fmt.Sprintf("failed to get forecast: %v", err)), nil
		}

		snap := result.Snapshot
		age := time.Since(snap.GeneratedAt).Round(time.Second)
		freshness := "fresh"
		if result.Stale || age > staleAfter {
			freshness = fmt.Sprintf("STALE (%s old)", age)
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Workload:        %s\n", snap.Workload)
		fmt.Fprintf(&sb, "Metric:          %s\n", snap.Metric)
		fmt.Fprintf(&sb, "Generated:       %s (%s, %s)\n", snap.GeneratedAt.Format(time.RFC3339), age, freshness)
		fmt.Fprintf(&sb, "Step:            %ds\n", snap.StepSeconds)
		fmt.Fprintf(&sb, "Horizon:         %ds (%d steps)\n", snap.HorizonSeconds, len(snap.Values))
		fmt.Fprintf(&sb, "\nForecast values (metric):\n  %s\n", formatFloats(snap.Values))
		fmt.Fprintf(&sb, "\nDesired replicas:\n  %s\n", formatInts(snap.DesiredReplicas))

		return mcp.NewToolResultText(sb.String()), nil
	}
}

func handleExplainDecision(fc *client.ForecasterClient, staleAfter time.Duration, log *slog.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workload, err := req.RequireString("workload")
		if err != nil {
			return mcp.NewToolResultError("workload parameter is required"), nil
		}

		result, err := fc.GetSnapshot(ctx, workload)
		if err != nil {
			log.Error("explain_decision failed", "workload", workload, "error", err)
			return mcp.NewToolResultError(fmt.Sprintf("failed to get forecast: %v", err)), nil
		}

		snap := result.Snapshot
		age := time.Since(snap.GeneratedAt).Round(time.Second)
		isStale := result.Stale || age > staleAfter

		var sb strings.Builder
		fmt.Fprintf(&sb, "Scaling decision for workload %q\n", snap.Workload)
		fmt.Fprintf(&sb, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

		if isStale {
			fmt.Fprintf(&sb, "WARNING: Forecast is stale (%s old). The scaler may be using outdated data.\n\n", age)
		} else {
			fmt.Fprintf(&sb, "Forecast age: %s (fresh)\n\n", age)
		}

		currentReplicas := 0
		if len(snap.DesiredReplicas) > 0 {
			currentReplicas = snap.DesiredReplicas[0]
		}

		peakReplicas, peakStep := maxReplicas(snap.DesiredReplicas)
		minReplicas, _ := minReplicasVal(snap.DesiredReplicas)
		peakTime := time.Duration(peakStep*snap.StepSeconds) * time.Second

		fmt.Fprintf(&sb, "Current recommendation: %d replicas\n", currentReplicas)
		fmt.Fprintf(&sb, "Peak over horizon:      %d replicas (at T+%s)\n", peakReplicas, peakTime)
		fmt.Fprintf(&sb, "Floor over horizon:     %d replicas\n\n", minReplicas)

		trend := analyzeTrend(snap.DesiredReplicas)
		fmt.Fprintf(&sb, "Trend: %s\n\n", trend)

		currentMetric := 0.0
		if len(snap.Values) > 0 {
			currentMetric = snap.Values[0]
		}
		fmt.Fprintf(&sb, "Current %s: %.2f\n", snap.Metric, currentMetric)
		fmt.Fprintf(&sb, "Forecast horizon: %d minutes ahead\n", snap.HorizonSeconds/60)

		return mcp.NewToolResultText(sb.String()), nil
	}
}

func maxReplicas(replicas []int) (max, idx int) {
	for i, v := range replicas {
		if v > max {
			max = v
			idx = i
		}
	}
	return max, idx
}

func minReplicasVal(replicas []int) (min, idx int) {
	if len(replicas) == 0 {
		return 0, 0
	}
	min = replicas[0]
	for i, v := range replicas {
		if v < min {
			min = v
			idx = i
		}
	}
	return min, idx
}

func analyzeTrend(replicas []int) string {
	if len(replicas) < 2 {
		return "stable (insufficient data)"
	}
	first := replicas[0]
	last := replicas[len(replicas)-1]
	mid := replicas[len(replicas)/2]

	switch {
	case last > first && mid >= first:
		return fmt.Sprintf("scaling up (%d → %d replicas over horizon)", first, last)
	case last < first && mid <= first:
		return fmt.Sprintf("scaling down (%d → %d replicas over horizon)", first, last)
	case mid > first && last <= first:
		return fmt.Sprintf("spike then scale down (%d → %d → %d replicas)", first, mid, last)
	case mid < first && last >= first:
		return fmt.Sprintf("dip then scale up (%d → %d → %d replicas)", first, mid, last)
	default:
		return fmt.Sprintf("stable around %d replicas", first)
	}
}

func formatFloats(vals []float64) string {
	if len(vals) == 0 {
		return "(none)"
	}
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = fmt.Sprintf("%.2f", v)
	}
	return strings.Join(parts, ", ")
}

func formatInts(vals []int) string {
	if len(vals) == 0 {
		return "(none)"
	}
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = fmt.Sprintf("%d", v)
	}
	return strings.Join(parts, ", ")
}
