# Configuration Reference

This document provides a complete reference for all configuration options in Kedastral.

## Configuration Modes

Kedastral supports two configuration modes:

### Single-Workload Mode (Legacy)
Configure a single workload using command-line flags or environment variables.

```bash
./bin/forecaster --workload=my-api --metric=http_rps --prom-query='...'
```

### Multi-Workload Mode
Configure multiple workloads using a YAML configuration file.

```bash
./bin/forecaster --config-file=workloads.yaml
```

See [Multi-Workload Configuration](#multi-workload-configuration) for details.

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

### Multi-Workload Configuration

Enable multi-workload mode with a YAML configuration file:

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--config-file` | `CONFIG_FILE` | _(empty)_ | Path to workloads YAML configuration file |

**Example workloads.yaml:**
```yaml
workloads:
  - name: api-frontend
    metric: http_rps
    prometheusURL: http://prometheus:9090
    prometheusQuery: 'sum(rate(http_requests_total{service="frontend"}[1m]))'
    horizon: 30m
    step: 1m
    interval: 30s
    window: 3h
    model: baseline
    targetPerPod: 100
    headroom: 1.2
    quantileLevel: p90  # or 0.90, or "0" to disable
    minReplicas: 2
    maxReplicas: 50
    upMaxFactorPerStep: 2.0
    downMaxPercentPerStep: 50

  - name: api-backend
    metric: http_rps
    prometheusURL: http://prometheus:9090
    prometheusQuery: 'sum(rate(http_requests_total{service="backend"}[1m]))'
    horizon: 1h
    step: 2m
    interval: 1m
    window: 6h
    model: arima
    arimaP: 2
    arimaD: 1
    arimaQ: 1
    targetPerPod: 200
    headroom: 1.3
    minReplicas: 3
    maxReplicas: 100
    upMaxFactorPerStep: 1.5
    downMaxPercentPerStep: 30
```

**Usage:**
```bash
./bin/forecaster --config-file=workloads.yaml
```

**Validation Rules:**
- Each workload must have a unique `name`
- `name` must be alphanumeric with dash/underscore, 1-253 characters
- `metric`, `prometheusQuery` are required
- `step` must be ≤ `horizon`
- `maxReplicas` must be ≥ `minReplicas`

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

### Production Setup (Multi-Workload + Redis + TLS)

```bash
# Start forecaster
./bin/forecaster \
  --config-file=/etc/kedastral/workloads.yaml \
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
