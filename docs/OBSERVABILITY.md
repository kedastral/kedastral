# Observability

Kedastral exposes Prometheus metrics, structured logging, and health endpoints for comprehensive observability.

## Metrics Endpoints

Both components expose Prometheus-compatible metrics:

- **Forecaster**: `http://<forecaster>:8081/metrics`
- **Scaler**: `http://<scaler>:8082/metrics`

## Forecaster Metrics

### Forecast Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `kedastral_predicted_value` | Gauge | `workload`, `metric` | Current predicted metric value at t+0 (e.g., RPS) |
| `kedastral_desired_replicas` | Gauge | `workload` | Current desired replica count (base forecast, before lead-time) |
| `kedastral_forecast_age_seconds` | Gauge | `workload` | Age of the current forecast in seconds |

**Example queries:**
```promql
# Current predicted RPS for my-api
kedastral_predicted_value{workload="my-api"}

# Desired replicas for all workloads
kedastral_desired_replicas

# Stale forecasts (older than 5 minutes)
kedastral_forecast_age_seconds > 300
```

### Performance Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `kedastral_adapter_collect_seconds` | Histogram | `workload`, `adapter` | Time spent collecting metrics from data source |
| `kedastral_model_predict_seconds` | Histogram | `workload`, `model` | Time spent generating forecast predictions |
| `kedastral_capacity_compute_seconds` | Histogram | `workload` | Time spent computing desired replicas from forecast |

**Example queries:**
```promql
# P95 adapter collection time
histogram_quantile(0.95, rate(kedastral_adapter_collect_seconds_bucket[5m]))

# P99 model prediction latency
histogram_quantile(0.99, rate(kedastral_model_predict_seconds_bucket[5m]))

# Average capacity computation time
rate(kedastral_capacity_compute_seconds_sum[5m]) / rate(kedastral_capacity_compute_seconds_count[5m])
```

### Error Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `kedastral_errors_total` | Counter | `workload`, `component`, `reason` | Total errors by component and failure reason |

**Component values:**
- `adapter`: Data collection failures
- `model`: Prediction failures
- `storage`: Storage backend failures

**Common reason values:**
- `timeout`: Operation timed out
- `connection_error`: Network/connection issue
- `invalid_data`: Bad data from source
- `storage_error`: Redis or memory store failure

**Example queries:**
```promql
# Error rate by component
rate(kedastral_errors_total[5m])

# Adapter errors for specific workload
kedastral_errors_total{workload="my-api", component="adapter"}

# Total storage failures
sum(rate(kedastral_errors_total{component="storage"}[5m]))
```

## Scaler Metrics

### Scaling Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `kedastral_scaler_desired_replicas_returned` | Gauge | `workload` | Last replica count returned to KEDA (includes lead-time logic) |
| `kedastral_scaler_forecast_age_seen_seconds` | Gauge | `workload` | Age of forecast when scaler last fetched it |

**Example queries:**
```promql
# Replicas returned to KEDA
kedastral_scaler_desired_replicas_returned{workload="my-api"}

# Forecast staleness at fetch time
kedastral_scaler_forecast_age_seen_seconds{workload="my-api"}
```

### Performance Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `kedastral_scaler_forecast_fetch_duration_seconds` | Histogram | `workload` | Time spent fetching forecast from forecaster |
| `kedastral_scaler_grpc_request_duration_seconds` | Histogram | `method` | KEDA gRPC request duration by method |

**Method values:**
- `IsActive`: KEDA checking if scaler is active
- `GetMetricSpec`: KEDA requesting metric specification
- `GetMetrics`: KEDA requesting current metric value

**Example queries:**
```promql
# P95 forecast fetch latency
histogram_quantile(0.95, rate(kedastral_scaler_forecast_fetch_duration_seconds_bucket[5m]))

# gRPC GetMetrics call rate
rate(kedastral_scaler_grpc_request_duration_seconds_count{method="GetMetrics"}[5m])

# P99 gRPC latency by method
histogram_quantile(0.99, rate(kedastral_scaler_grpc_request_duration_seconds_bucket[5m])) by (method)
```

### Error Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `kedastral_scaler_forecast_fetch_errors_total` | Counter | `workload`, `reason` | Total errors fetching forecasts from forecaster |

**Reason values:**
- `http_error`: HTTP request failed
- `decode_error`: Invalid JSON response
- `not_found`: Workload not found
- `stale`: Forecast too old

**Example queries:**
```promql
# Forecast fetch error rate
rate(kedastral_scaler_forecast_fetch_errors_total[5m])

# HTTP errors for specific workload
kedastral_scaler_forecast_fetch_errors_total{workload="my-api", reason="http_error"}
```

## Health Endpoints

### Forecaster Health

**Endpoint:** `GET /healthz`

**Response:**
- `200 OK`: Service is healthy
- `503 Service Unavailable`: Service is unhealthy

**Health checks:**
- HTTP server is running
- Storage backend is accessible (Redis ping if configured)
- At least one forecast generated successfully (after startup)

**Example:**
```bash
curl http://kedastral-forecaster:8081/healthz
# HTTP 200 OK
```

### Scaler Health

**Endpoint:** `GET /healthz`

**Response:**
- `200 OK`: Service is healthy
- `503 Service Unavailable`: Service is unhealthy

**Health checks:**
- HTTP server is running
- gRPC server is running
- Forecaster endpoint is reachable

**Example:**
```bash
curl http://kedastral-scaler:8082/healthz
# HTTP 200 OK
```

## Structured Logging

### Log Levels

Both components support structured logging with configurable levels:

- `debug`: Verbose debugging information
- `info`: Informational messages (default)
- `warn`: Warning messages
- `error`: Error messages

**Configuration:**
```bash
# Text format (human-readable)
--log-level=info --log-format=text

# JSON format (machine-readable)
--log-level=debug --log-format=json
```

### Log Fields

**Common fields:**
- `time`: Timestamp (RFC3339)
- `level`: Log level
- `msg`: Log message
- `component`: Component name (`forecaster` or `scaler`)

**Forecaster-specific:**
- `workload`: Workload name
- `metric`: Metric name
- `predicted_value`: Predicted metric value
- `desired_replicas`: Desired replica count
- `duration_ms`: Operation duration

**Scaler-specific:**
- `workload`: Workload name
- `method`: gRPC method called
- `replicas_returned`: Replicas returned to KEDA
- `forecast_age_s`: Forecast age in seconds

### Example Logs

**Text format:**
```
2025-12-22T10:15:30Z info forecaster: generated forecast workload=my-api predicted_value=450.2 desired_replicas=5
2025-12-22T10:15:35Z info scaler: returned replicas to KEDA workload=my-api replicas=6 forecast_age_s=5
2025-12-22T10:15:40Z warn forecaster: slow adapter collection workload=my-api duration_ms=2500
2025-12-22T10:15:45Z error scaler: forecast fetch failed workload=my-api error="http 503"
```

**JSON format:**
```json
{"time":"2025-12-22T10:15:30Z","level":"info","component":"forecaster","msg":"generated forecast","workload":"my-api","predicted_value":450.2,"desired_replicas":5}
{"time":"2025-12-22T10:15:35Z","level":"info","component":"scaler","msg":"returned replicas to KEDA","workload":"my-api","replicas":6,"forecast_age_s":5}
{"time":"2025-12-22T10:15:40Z","level":"warn","component":"forecaster","msg":"slow adapter collection","workload":"my-api","duration_ms":2500}
{"time":"2025-12-22T10:15:45Z","level":"error","component":"scaler","msg":"forecast fetch failed","workload":"my-api","error":"http 503"}
```

## Prometheus ServiceMonitor

To scrape metrics automatically with Prometheus Operator:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: kedastral-forecaster
  namespace: default
spec:
  selector:
    matchLabels:
      component: forecaster
  endpoints:
    - port: http
      path: /metrics
      interval: 30s
---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: kedastral-scaler
  namespace: default
spec:
  selector:
    matchLabels:
      component: scaler
  endpoints:
    - port: http
      path: /metrics
      interval: 30s
```

## Grafana Dashboards

Kedastral provides pre-built Grafana dashboards (planned for v0.2):

### Dashboard 1: Forecast Overview

**Panels:**
- Predicted metric values over time
- Desired vs actual replicas
- Forecast accuracy (if actual metrics available)
- Forecast age and staleness

### Dashboard 2: Performance

**Panels:**
- Adapter collection latency (P50, P95, P99)
- Model prediction latency
- Capacity computation latency
- gRPC request duration

### Dashboard 3: Errors & Health

**Panels:**
- Error rate by component
- Error breakdown by reason
- Health check status
- Forecast fetch success rate

## Alerting

### Recommended Alerts

**Stale Forecasts:**
```yaml
- alert: StaleForecasts
  expr: kedastral_forecast_age_seconds > 300
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "Forecast for {{ $labels.workload }} is stale"
    description: "Forecast age is {{ $value }}s (>5m)"
```

**High Error Rate:**
```yaml
- alert: HighErrorRate
  expr: rate(kedastral_errors_total[5m]) > 0.1
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "High error rate in {{ $labels.component }}"
    description: "Error rate is {{ $value }}/s"
```

**Scaler Fetch Failures:**
```yaml
- alert: ScalerFetchFailures
  expr: rate(kedastral_scaler_forecast_fetch_errors_total[5m]) > 0.1
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "Scaler failing to fetch forecasts for {{ $labels.workload }}"
    description: "Error rate is {{ $value }}/s, reason: {{ $labels.reason }}"
```

**Slow Predictions:**
```yaml
- alert: SlowPredictions
  expr: histogram_quantile(0.95, rate(kedastral_model_predict_seconds_bucket[5m])) > 1.0
  for: 10m
  labels:
    severity: warning
  annotations:
    summary: "Model predictions are slow for {{ $labels.workload }}"
    description: "P95 latency is {{ $value }}s (>1s)"
```

**Forecaster Down:**
```yaml
- alert: ForecasterDown
  expr: up{job="kedastral-forecaster"} == 0
  for: 2m
  labels:
    severity: critical
  annotations:
    summary: "Forecaster is down"
    description: "Forecaster has been unavailable for 2 minutes"
```

**Scaler Down:**
```yaml
- alert: ScalerDown
  expr: up{job="kedastral-scaler"} == 0
  for: 2m
  labels:
    severity: critical
  annotations:
    summary: "Scaler is down"
    description: "Scaler has been unavailable for 2 minutes"
```

## Tracing (Future)

Planned for future releases:

- OpenTelemetry integration
- Distributed tracing across forecaster → scaler → KEDA
- Trace sampling and export to Jaeger/Tempo

## Custom Metrics Export

For advanced observability, you can export Kedastral metrics to:

- **Datadog**: Via Prometheus integration
- **New Relic**: Via Prometheus remote write
- **CloudWatch**: Via CloudWatch Agent
- **Custom backends**: Via Prometheus remote write protocol

## Next Steps

- **Deployment**: See [DEPLOYMENT.md](DEPLOYMENT.md) for monitoring setup
- **Configuration**: See [CONFIGURATION.md](CONFIGURATION.md) for log configuration
- **Troubleshooting**: See [examples/README.md](../examples/README.md) for common issues
