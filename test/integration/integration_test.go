//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/HatiCode/kedastral/pkg/api/externalscaler"
)

// TestForecasterScalerE2E tests the complete integration using real containers
func TestForecasterScalerE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Create a custom network for container-to-container communication
	networkName := "kedastral-test"
	network, err := testcontainers.GenericNetwork(ctx, testcontainers.GenericNetworkRequest{
		NetworkRequest: testcontainers.NetworkRequest{
			Name:           networkName,
			CheckDuplicate: true,
		},
	})
	if err != nil {
		t.Fatalf("Failed to create network: %v", err)
	}
	defer network.Remove(ctx)

	// 1. Start a mock Prometheus server using Python's built-in HTTP server
	// This returns a matrix result with multiple time series points
	now := time.Now().Unix()
	promResponse := fmt.Sprintf(`{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"service":"test-api"},"values":[[%d,"100.0"],[%d,"110.0"],[%d,"120.0"],[%d,"130.0"],[%d,"140.0"]]}]}}`, now-240, now-180, now-120, now-60, now)

	// Create a simple Python HTTP server that mimics Prometheus API
	pythonScript := `
import http.server
import socketserver

class PrometheusHandler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        # Accept any path with query parameter (Prometheus range queries) or /api/v1/query
        if '?' in self.path or '/api/v1/query' in self.path:
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            self.wfile.write(b'` + promResponse + `')
        else:
            self.send_response(404)
            self.end_headers()

    def log_message(self, format, *args):
        pass  # Suppress HTTP logs

PORT = 9090
with socketserver.TCPServer(("", PORT), PrometheusHandler) as httpd:
    httpd.serve_forever()
`

	promReq := testcontainers.ContainerRequest{
		Image:        "python:3.11-alpine",
		ExposedPorts: []string{"9090/tcp"},
		Cmd:          []string{"python", "-c", pythonScript},
		Networks:     []string{networkName},
		NetworkAliases: map[string][]string{
			networkName: {"prometheus"},
		},
		WaitingFor: wait.ForListeningPort("9090/tcp").WithStartupTimeout(30 * time.Second),
	}

	promContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: promReq,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to start Prometheus mock container: %v", err)
	}
	defer promContainer.Terminate(ctx)

	// Use the network alias for container-to-container communication
	promURL := "http://prometheus:9090"

	// 2. Build and start the forecaster container
	forecasterReq := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    "../../",
			Dockerfile: "Dockerfile.forecaster",
		},
		ExposedPorts: []string{"8081/tcp"},
		Networks:     []string{networkName},
		NetworkAliases: map[string][]string{
			networkName: {"forecaster"},
		},
		Cmd: []string{
			"-workload=test-api",
			"-metric=http_rps",
			"-prom-url=" + promURL,
			"-prom-query=sum(rate(http_requests_total{service=\"test-api\"}[1m]))",
			"-target-per-pod=50",
			"-headroom=1.2",
			"-min=2",
			"-max=20",
			"-horizon=6m",
			"-step=1m",
			"-interval=5s",
			"-window=5m",
			"-log-level=debug",
		},
		WaitingFor: wait.ForHTTP("/healthz").WithPort("8081/tcp").WithStartupTimeout(60 * time.Second),
	}

	forecasterContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: forecasterReq,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to start forecaster container: %v", err)
	}
	defer forecasterContainer.Terminate(ctx)

	forecasterHost, err := forecasterContainer.Host(ctx)
	if err != nil {
		t.Fatalf("Failed to get forecaster host: %v", err)
	}

	forecasterPort, err := forecasterContainer.MappedPort(ctx, "8081")
	if err != nil {
		t.Fatalf("Failed to get forecaster port: %v", err)
	}

	forecasterURL := fmt.Sprintf("http://%s:%s", forecasterHost, forecasterPort.Port())

	// Wait for the forecaster to generate at least one forecast
	time.Sleep(15 * time.Second)

	// Verify forecaster has a forecast
	resp, err := http.Get(forecasterURL + "/forecast/current?workload=test-api")
	if err != nil {
		t.Fatalf("Failed to fetch forecast from forecaster: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Print forecaster container logs
		logs, logErr := forecasterContainer.Logs(ctx)
		if logErr == nil {
			defer logs.Close()
			logBytes, _ := io.ReadAll(logs)
			t.Logf("Forecaster container logs:\n%s", string(logBytes))
		}

		// Print Prometheus container logs to see if it received requests
		promLogs, promLogErr := promContainer.Logs(ctx)
		if promLogErr == nil {
			defer promLogs.Close()
			promLogBytes, _ := io.ReadAll(promLogs)
			t.Logf("Prometheus container logs:\n%s", string(promLogBytes))
		}

		t.Fatalf("Forecaster returned non-OK status: %d", resp.StatusCode)
	}

	var forecastCheck map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&forecastCheck); err != nil {
		t.Fatalf("Failed to decode forecast: %v", err)
	}

	// 3. Build and start the scaler container

	// Use the forecaster's network alias for container-to-container communication
	forecasterNetworkURL := "http://forecaster:8081"

	scalerReq := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    "../../",
			Dockerfile: "Dockerfile.scaler",
		},
		ExposedPorts: []string{"50051/tcp", "8082/tcp"},
		Networks:     []string{networkName},
		Cmd: []string{
			"-forecaster-url=" + forecasterNetworkURL,
			"-lead-time=5m",
			"-log-level=debug",
		},
		WaitingFor: wait.ForHTTP("/healthz").WithPort("8082/tcp").WithStartupTimeout(60 * time.Second),
	}

	scalerContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: scalerReq,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to start scaler container: %v", err)
	}
	defer scalerContainer.Terminate(ctx)

	scalerHost, err := scalerContainer.Host(ctx)
	if err != nil {
		t.Fatalf("Failed to get scaler host: %v", err)
	}

	scalerGRPCPort, err := scalerContainer.MappedPort(ctx, "50051")
	if err != nil {
		t.Fatalf("Failed to get scaler gRPC port: %v", err)
	}

	scalerGRPCAddr := fmt.Sprintf("%s:%s", scalerHost, scalerGRPCPort.Port())

	// 4. Connect to the scaler via gRPC (simulating KEDA)
	conn, err := grpc.NewClient(
		scalerGRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("Failed to connect to scaler: %v", err)
	}
	defer conn.Close()

	client := pb.NewExternalScalerClient(conn)

	// 5. Test IsActive
	t.Run("IsActive", func(t *testing.T) {
		ref := &pb.ScaledObjectRef{
			Name:      "test-scaledobject",
			Namespace: "default",
			ScalerMetadata: map[string]string{
				"workload": "test-api",
			},
		}

		resp, err := client.IsActive(ctx, ref)
		if err != nil {
			t.Fatalf("IsActive failed: %v", err)
		}

		if !resp.Result {
			t.Error("Expected scaler to be active, but got inactive")
		}
		t.Log("✓ Scaler is active")
	})

	// 6. Test GetMetricSpec
	t.Run("GetMetricSpec", func(t *testing.T) {
		ref := &pb.ScaledObjectRef{
			Name:      "test-scaledobject",
			Namespace: "default",
			ScalerMetadata: map[string]string{
				"workload":   "test-api",
				"metricName": "kedastral-test-api-desired-replicas",
			},
		}

		resp, err := client.GetMetricSpec(ctx, ref)
		if err != nil {
			t.Fatalf("GetMetricSpec failed: %v", err)
		}

		if len(resp.MetricSpecs) != 1 {
			t.Fatalf("Expected 1 metric spec, got %d", len(resp.MetricSpecs))
		}

		spec := resp.MetricSpecs[0]
		if spec.MetricName != "kedastral-test-api-desired-replicas" {
			t.Errorf("Expected metric name 'kedastral-test-api-desired-replicas', got '%s'", spec.MetricName)
		}
		t.Logf("✓ Metric spec: %s (target: %.0f)", spec.MetricName, spec.TargetSizeFloat)
	})

	// 7. Test GetMetrics - the main integration test
	t.Run("GetMetrics", func(t *testing.T) {
		req := &pb.GetMetricsRequest{
			ScaledObjectRef: &pb.ScaledObjectRef{
				Name:      "test-scaledobject",
				Namespace: "default",
				ScalerMetadata: map[string]string{
					"workload": "test-api",
				},
			},
			MetricName: "kedastral-test-api-desired-replicas",
		}

		resp, err := client.GetMetrics(ctx, req)
		if err != nil {
			t.Fatalf("GetMetrics failed: %v", err)
		}

		if len(resp.MetricValues) != 1 {
			t.Fatalf("Expected 1 metric value, got %d", len(resp.MetricValues))
		}

		metric := resp.MetricValues[0]
		if metric.MetricName != "kedastral-test-api-desired-replicas" {
			t.Errorf("Expected metric name 'kedastral-test-api-desired-replicas', got '%s'", metric.MetricName)
		}

		// Verify we got a reasonable replica count
		// With RPS increasing from 100 to 200+, target 50 per pod, headroom 1.2
		// We should get at least 2 replicas (min) and likely more
		if metric.MetricValueFloat < 2.0 {
			t.Errorf("Expected at least 2 replicas (min), got %v", metric.MetricValueFloat)
		}

		if metric.MetricValueFloat > 20.0 {
			t.Errorf("Expected at most 20 replicas (max), got %v", metric.MetricValueFloat)
		}

		t.Logf("✓ Scaler returned %v desired replicas", metric.MetricValueFloat)
	})

	// 8. Test with unknown workload
	t.Run("GetMetrics_UnknownWorkload", func(t *testing.T) {
		req := &pb.GetMetricsRequest{
			ScaledObjectRef: &pb.ScaledObjectRef{
				Name:      "unknown-scaledobject",
				Namespace: "default",
				ScalerMetadata: map[string]string{
					"workload": "unknown-workload",
				},
			},
			MetricName: "kedastral-unknown-desired-replicas",
		}

		_, err := client.GetMetrics(ctx, req)
		if err == nil {
			t.Error("Expected error for unknown workload, got nil")
		} else {
			t.Logf("✓ Correctly returned error for unknown workload: %v", err)
		}
	})

	t.Log("✓ All integration tests passed!")
}
