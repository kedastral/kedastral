# Configuration Reference

This document provides a complete reference for all configuration options in Kedastral.

## Architecture: One Workload Per Deployment

**Security Note:** Kedastral follows a single-workload-per-deployment architecture for security and isolation. Each forecaster instance manages exactly one workload configured via flags or environment variables.

For managing multiple workloads, deploy multiple forecaster instances (recommended pattern: use Helm to template multiple deployments).

```bash
# Each workload gets its own forecaster deployment
./bin/forecaster --workload=my-api --metric=http_rps --prom-query='...'
./bin/forecaster --workload=batch-jobs --metric=queue_depth --prom-query='...'
```

## Configuration Precedence

Configuration values are resolved in the following order (highest priority first):

1. **Command-line flags** (e.g., `--workload=my-api`)
2. **Environment variables** (e.g., `WORKLOAD=my-api`)
3. **Default values**

## Forecaster Configuration

### Server Settings

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--listen` | `LISTEN` | `:8081` | HTTP listen address for REST API and metrics |

**Example:**
```bash
./bin/forecaster --listen=:8080
```

### Logging

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--log-format` | `LOG_FORMAT` | `text` | Log output format: `text` or `json` |
| `--log-level` | `LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |

**Example:**
```bash
./bin/forecaster --log-level=debug --log-format=json
```

### Workload Identification

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--workload` | `WORKLOAD` | _(required)_ | Workload name (alphanumeric with dash/underscore, 1-253 chars) |
| `--metric` | `METRIC` | _(required)_ | Metric name being forecasted (e.g., `http_rps`, `queue_depth`) |

**Example:**
```bash
./bin/forecaster --workload=my-api --metric=http_rps
```

### Forecast Parameters

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--horizon` | `HORIZON` | `30m` | Forecast horizon (how far into the future to predict) |
| `--step` | `STEP` | `1m` | Forecast step size (time between prediction points) |
| `--interval` | `INTERVAL` | `30s` | How often to generate new forecasts |
| `--window` | `WINDOW` | `30m` | Historical data window for model training |

**Notes:**
- `step` must be ≤ `horizon`
- Longer `horizon` provides more lookahead but may be less accurate
- Smaller `step` provides finer granularity but more data points

**Example:**
```bash
./bin/forecaster --horizon=1h --step=2m --interval=1m --window=3h
```

### Model Selection

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--model` | `MODEL` | `baseline` | Forecasting model: `baseline` or `arima` |
| `--arima-p` | `ARIMA_P` | `0` (auto) | ARIMA AR order (1-3 typical, 0=auto defaults to 1) |
| `--arima-d` | `ARIMA_D` | `0` (auto) | ARIMA differencing order (0-2, 0=auto defaults to 1) |
| `--arima-q` | `ARIMA_Q` | `0` (auto) | ARIMA MA order (1-3 typical, 0=auto defaults to 1) |

**Model Comparison:**

| Model | Training | Startup | Best For |
|-------|----------|---------|----------|
| `baseline` | None | Immediate | Stable workloads with basic patterns |
| `arima` | Required | Warm-up needed | Complex patterns with trends/seasonality |

**Example (Baseline):**
```bash
./bin/forecaster --model=baseline
```

**Example (ARIMA):**
```bash
# Auto parameters (ARIMA(1,1,1))
./bin/forecaster --model=arima

# Custom parameters
./bin/forecaster --model=arima --arima-p=2 --arima-d=1 --arima-q=2
```

See [models/](models/) for detailed model documentation.

### Capacity Planning Policy

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--target-per-pod` | `TARGET_PER_POD` | `100.0` | Target metric value per pod (e.g., 100 RPS per pod) |
| `--headroom` | `HEADROOM` | `1.2` | Headroom multiplier for safety buffer (1.2 = 20% buffer) |
| `--quantile-level` | `QUANTILE_LEVEL` | `0` | Quantile level for planning (e.g., `p90`, `0.90`). Set to `0` to disable and use headroom. |
| `--min` | `MIN_REPLICAS` | `1` | Minimum replica count |
| `--max` | `MAX_REPLICAS` | `100` | Maximum replica count |
| `--up-max-factor` | `UP_MAX_FACTOR` | `2.0` | Maximum scale-up factor per step (2.0 = can double) |
| `--down-max-percent` | `DOWN_MAX_PERCENT` | `50` | Maximum scale-down percent per step (50 = can halve) |

**Capacity Formula:**
```
rawReplicas = predictedMetric / targetPerPod
adjustedReplicas = rawReplicas * headroom  (or use quantile if enabled)
finalReplicas = clamp(adjustedReplicas, min, max, upFactor, downPercent)
```

**Example:**
```bash
./bin/forecaster \
  --target-per-pod=200 \
  --headroom=1.3 \
  --min=2 \
  --max=50 \
  --up-max-factor=1.5 \
  --down-max-percent=20
```

For detailed tuning guidance, see [planner/tuning.md](planner/tuning.md).

### Prometheus Adapter

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--prom-url` | `PROM_URL` | `http://localhost:9090` | Prometheus server URL |
| `--prom-query` | `PROM_QUERY` | _(required)_ | PromQL query to fetch metric data |

**Example:**
```bash
./bin/forecaster \
  --prom-url=http://prometheus:9090 \
  --prom-query='sum(rate(http_requests_total{service="my-api"}[1m]))'
```

**Query Guidelines:**
- Use `rate()` or `irate()` for counter metrics
- Use `sum()` to aggregate across pods
- Use appropriate time ranges (e.g., `[1m]` for per-minute rates)

See [adapters/prometheus-limits.md](adapters/prometheus-limits.md) for query optimization.

### VictoriaMetrics Adapter

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--victoria-metrics-url` | `VICTORIA_METRICS_URL` | `http://localhost:8428` | VictoriaMetrics server URL |
| `--victoria-metrics-query` | `VICTORIA_METRICS_QUERY` | _(required)_ | PromQL/MetricsQL query to fetch metric data |

**Example (Single-Node):**
```bash
./bin/forecaster \
  --victoria-metrics-url=http://victoria-metrics:8428 \
  --victoria-metrics-query='sum(rate(http_requests_total{service="my-api"}[1m]))'
```

**Example (Cluster):**
```bash
./bin/forecaster \
  --victoria-metrics-url=http://vmselect:8481/select/0/prometheus \
  --victoria-metrics-query='sum(rollup_rate(http_requests_total{service="my-api"}[5m]))'
```

**VictoriaMetrics Features:**
- **Prometheus-compatible API**: All PromQL queries work unchanged
- **MetricsQL extensions**: Use `rollup_rate()`, `default_if_empty()`, `quantile_over_time()`, etc.
- **Better performance**: Faster queries and lower resource usage than Prometheus
- **Multi-tenancy**: Use `/select/<accountID>/prometheus` path for multi-tenant setups
- **Long-term retention**: Efficient storage for extended historical data

**Common VictoriaMetrics Ports:**
- Single-node: `8428`
- Cluster vmselect: `8481`

**MetricsQL Query Examples:**
```bash
# Use MetricsQL rollup_rate (more accurate than rate)
--victoria-metrics-query='sum(rollup_rate(requests_total[5m]))'

# Handle missing metrics gracefully
--victoria-metrics-query='default_if_empty(sum(queue_depth), 0)'

# Built-in quantile calculation
--victoria-metrics-query='quantile_over_time(0.99, response_time[5m])'
```

See [deploy/examples/workloads-victoriametrics.yaml](../deploy/examples/workloads-victoriametrics.yaml) for comprehensive examples.

### Storage Backend

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--storage` | `STORAGE` | `memory` | Storage backend: `memory` or `redis` |
| `--redis-addr` | `REDIS_ADDR` | `localhost:6379` | Redis server address (host:port) |
| `--redis-password` | `REDIS_PASSWORD` | _(empty)_ | Redis authentication password |
| `--redis-db` | `REDIS_DB` | `0` | Redis database number (0-15) |
| `--redis-ttl` | `REDIS_TTL` | `30m` | Snapshot TTL in Redis |

**In-Memory Storage (Default):**
```bash
./bin/forecaster --storage=memory
```

**Redis Storage (HA):**
```bash
./bin/forecaster \
  --storage=redis \
  --redis-addr=redis:6379 \
  --redis-password=secret \
  --redis-db=0 \
  --redis-ttl=1h
```

### TLS Configuration

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--tls-enabled` | `TLS_ENABLED` | `false` | Enable TLS for HTTP server |
| `--tls-cert-file` | `TLS_CERT_FILE` | _(empty)_ | Path to TLS certificate file |
| `--tls-key-file` | `TLS_KEY_FILE` | _(empty)_ | Path to TLS private key file |
| `--tls-ca-file` | `TLS_CA_FILE` | _(empty)_ | Path to CA certificate for client verification |

**Example:**
```bash
./bin/forecaster \
  --tls-enabled \
  --tls-cert-file=/etc/certs/server.crt \
  --tls-key-file=/etc/certs/server.key \
  --tls-ca-file=/etc/certs/ca.crt
```

### Managing Multiple Workloads

To manage multiple workloads, deploy multiple forecaster instances. The recommended pattern is using Helm to template multiple deployments.

**Helm values.yaml:**
```yaml
workloads:
  - name: api-frontend
    metric: http_rps
    promQuery: 'sum(rate(http_requests_total{service="frontend"}[1m]))'
    model: baseline
    targetPerPod: 100
    headroom: 1.2
    minReplicas: 2
    maxReplicas: 50

  - name: api-backend
    metric: http_rps
    promQuery: 'sum(rate(http_requests_total{service="backend"}[1m]))'
    model: sarima
    sarimaS: 24
    targetPerPod: 200
    minReplicas: 3
    maxReplicas: 100

  - name: batch-processor
    metric: queue_depth
    victoriaMetricsQuery: 'kafka_consumer_lag_seconds'
    model: arima
    arimaP: 2
    targetPerPod: 50
    minReplicas: 1
    maxReplicas: 20
```

**Helm template (templates/forecaster-deployment.yaml):**
```yaml
{{- range .Values.workloads }}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kedastral-forecaster-{{ .name }}
  labels:
    app: kedastral
    component: forecaster
    workload: {{ .name }}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: kedastral
      component: forecaster
      workload: {{ .name }}
  template:
    metadata:
      labels:
        app: kedastral
        component: forecaster
        workload: {{ .name }}
    spec:
      containers:
      - name: forecaster
        image: kedastral/forecaster:latest
        env:
        - name: WORKLOAD
          value: {{ .name | quote }}
        - name: METRIC
          value: {{ .metric | quote }}
        - name: MODEL
          value: {{ .model | default "baseline" | quote }}
        {{- if .promQuery }}
        - name: PROM_QUERY
          value: {{ .promQuery | quote }}
        {{- end }}
        {{- if .victoriaMetricsQuery }}
        - name: VICTORIA_METRICS_QUERY
          value: {{ .victoriaMetricsQuery | quote }}
        {{- end }}
        - name: TARGET_PER_POD
          value: {{ .targetPerPod | default 100 | quote }}
        - name: MIN_REPLICAS
          value: {{ .minReplicas | default 1 | quote }}
        - name: MAX_REPLICAS
          value: {{ .maxReplicas | default 100 | quote }}
        # Add other env vars as needed
        ports:
        - containerPort: 8081
          name: http
---
apiVersion: v1
kind: Service
metadata:
  name: kedastral-forecaster-{{ .name }}
  labels:
    app: kedastral
    component: forecaster
    workload: {{ .name }}
spec:
  ports:
  - port: 8081
    targetPort: 8081
    name: http
  selector:
    app: kedastral
    component: forecaster
    workload: {{ .name }}
{{- end }}
```

**Benefits of this approach:**
- **Security:** No file system access, each workload isolated
- **Scalability:** Scale forecasters independently per workload
- **Reliability:** Failure isolation - one workload failure doesn't affect others
- **Kubernetes-native:** Standard pattern using ConfigMaps/env vars

## Scaler Configuration

### Server Settings

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--listen` | `SCALER_LISTEN` | `:50051` | gRPC listen address for KEDA External Scaler |

**Example:**
```bash
./bin/scaler --listen=:8080
```

### Forecaster Connection

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--forecaster-url` | `FORECASTER_URL` | `http://localhost:8081` | Forecaster HTTP endpoint URL |

**Example:**
```bash
./bin/scaler --forecaster-url=http://kedastral-forecaster:8081
```

### Lead Time Configuration

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--lead-time` | `LEAD_TIME` | `5m` | How far ahead to look in forecast for proactive scaling |

**Lead Time Guidelines:**

| Lead Time | Use Case | Behavior |
|-----------|----------|----------|
| `3m-5m` | Gradual changes, responsive workloads | Moderate proactivity |
| `10m-15m` | Predictable spikes, batch jobs | High proactivity |
| `20m+` | Very slow scaling (heavy containers) | Maximum proactivity |

**Example:**
```bash
./bin/scaler --lead-time=10m
```

### Logging

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--log-format` | `LOG_FORMAT` | `text` | Log output format: `text` or `json` |
| `--log-level` | `LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |

**Example:**
```bash
./bin/scaler --log-level=debug --log-format=json
```

### TLS Configuration

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--tls-enabled` | `TLS_ENABLED` | `false` | Enable TLS for HTTP client (forecaster connection) |
| `--tls-cert-file` | `TLS_CERT_FILE` | _(empty)_ | Path to TLS certificate file |
| `--tls-key-file` | `TLS_KEY_FILE` | _(empty)_ | Path to TLS private key file |
| `--tls-ca-file` | `TLS_CA_FILE` | _(empty)_ | Path to CA certificate for server verification |

**Example:**
```bash
./bin/scaler \
  --tls-enabled \
  --tls-cert-file=/etc/certs/client.crt \
  --tls-key-file=/etc/certs/client.key \
  --tls-ca-file=/etc/certs/ca.crt
```

## Complete Examples

### Development Setup (Single Workload)

```bash
# Start forecaster
./bin/forecaster \
  --workload=my-api \
  --metric=http_rps \
  --prom-url=http://localhost:9090 \
  --prom-query='sum(rate(http_requests_total{service="my-api"}[1m]))' \
  --target-per-pod=100 \
  --headroom=1.2 \
  --min=2 \
  --max=50 \
  --horizon=30m \
  --step=1m \
  --interval=30s \
  --window=3h \
  --model=baseline \
  --log-level=debug

# Start scaler
./bin/scaler \
  --forecaster-url=http://localhost:8081 \
  --lead-time=5m \
  --log-level=debug
```

### Production Setup (Redis + TLS)

```bash
# Start forecaster
./bin/forecaster \
  --workload=my-api \
  --metric=http_rps \
  --prom-url=http://prometheus:9090 \
  --prom-query='sum(rate(http_requests_total{service="my-api"}[1m]))' \
  --model=sarima \
  --sarima-s=24 \
  --storage=redis \
  --redis-addr=redis:6379 \
  --redis-password=secret \
  --redis-ttl=1h \
  --tls-enabled \
  --tls-cert-file=/etc/certs/server.crt \
  --tls-key-file=/etc/certs/server.key \
  --tls-ca-file=/etc/certs/ca.crt \
  --log-level=info \
  --log-format=json

# Start scaler
./bin/scaler \
  --forecaster-url=https://kedastral-forecaster:8081 \
  --lead-time=10m \
  --tls-enabled \
  --tls-cert-file=/etc/certs/client.crt \
  --tls-key-file=/etc/certs/client.key \
  --tls-ca-file=/etc/certs/ca.crt \
  --log-level=info \
  --log-format=json
```

**Note:** For multiple workloads in production, deploy multiple forecaster instances using Helm (see [Managing Multiple Workloads](#managing-multiple-workloads)).

## Environment Variables Only

If you prefer environment variables over flags:

```bash
# Forecaster
export WORKLOAD=my-api
export METRIC=http_rps
export PROM_URL=http://prometheus:9090
export PROM_QUERY='sum(rate(http_requests_total[1m]))'
export TARGET_PER_POD=100
export HEADROOM=1.2
export MIN_REPLICAS=2
export MAX_REPLICAS=50
export STORAGE=redis
export REDIS_ADDR=redis:6379
./bin/forecaster

# Scaler
export FORECASTER_URL=http://kedastral-forecaster:8081
export LEAD_TIME=10m
./bin/scaler
```

## Kubernetes ConfigMap

For Kubernetes deployments, use a ConfigMap for non-sensitive configuration:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kedastral-config
data:
  WORKLOAD: "my-api"
  METRIC: "http_rps"
  PROM_URL: "http://prometheus:9090"
  TARGET_PER_POD: "100"
  HEADROOM: "1.2"
  MIN_REPLICAS: "2"
  MAX_REPLICAS: "50"
  STORAGE: "redis"
  REDIS_ADDR: "redis:6379"
  LEAD_TIME: "10m"
```

Use Secrets for sensitive data:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: kedastral-secrets
stringData:
  REDIS_PASSWORD: "your-redis-password"
  PROM_QUERY: 'sum(rate(http_requests_total{service="my-api"}[1m]))'
```

## Next Steps

- **Quick Start**: See [QUICKSTART.md](QUICKSTART.md) for deployment examples
- **Tuning**: See [planner/tuning.md](planner/tuning.md) for capacity planning optimization
- **Models**: See [models/](models/) for model selection and configuration
- **Production**: See [DEPLOYMENT.md](DEPLOYMENT.md) for production considerations
