# Forecaster

The Kedastral Forecaster is the prediction engine that collects metrics, generates forecasts, and computes desired replica counts.

## What It Does

The forecaster runs a continuous loop that:

1. **Collects** recent metrics from Prometheus (or other adapters)
2. **Warm-starts** by loading historical data on startup for immediate predictions
3. **Predicts** future metric values using statistical or ML models
4. **Plans** capacity by converting predictions to desired replica counts
5. **Stores** forecast snapshots in memory or Redis
6. **Exposes** forecasts via HTTP API for the scaler to consume

## Architecture

```
┌─────────────┐
│ Prometheus  │
└──────┬──────┘
       │ Metrics
       ▼
┌─────────────────────────────┐
│     Forecaster Loop         │
│                             │
│  1. Collect (Adapter)       │
│  2. Engineer Features       │
│  3. Predict (Model)         │
│  4. Compute Replicas        │
│  5. Store Snapshot          │
└──────────┬──────────────────┘
           │
           ▼
    ┌──────────────┐
    │ Storage      │
    │ (Memory/Redis)│
    └──────┬───────┘
           │
           ▼
    ┌──────────────┐
    │  HTTP API    │
    │  /forecast/  │
    │   current    │
    └──────────────┘
```

## Key Concepts

### Warm Start

On startup, the forecaster queries Prometheus for the full historical window (e.g., 3 hours) to immediately learn seasonal patterns, rather than waiting for data to accumulate.

**Benefits:**
- Immediate accurate predictions
- No cold-start period
- Learns from existing patterns

### Forecast Horizon

The **horizon** determines how far into the future predictions are made. A 30-minute horizon with 1-minute steps produces 30 prediction points.

**Considerations:**
- Longer horizon = more lookahead but less accuracy
- Shorter horizon = more accurate but less warning time
- Typical: 30m-1h for most workloads

### Capacity Planning

The forecaster converts predicted metric values into desired replica counts using a configurable policy:

```go
rawReplicas = predictedValue / targetPerPod
adjustedReplicas = rawReplicas * headroom  // or quantile
finalReplicas = clamp(adjusted, min, max, upFactor, downPercent)
```

See [../../docs/planner/](../../docs/planner/) for details.

## Models

### Baseline (Default)

Statistical model with:
- Linear trend detection
- Momentum calculation
- Multi-level seasonality (minute-of-hour, hour-of-day, day-of-week)
- Adaptive blending

**Best for:**
- Workloads with recurring patterns
- Fast startup required
- Low resource overhead

**Configuration:**
```bash
--model=baseline
```

See [../../docs/models/baseline.md](../../docs/models/baseline.md) for details.

### ARIMA

Time-series forecasting with AutoRegressive Integrated Moving Average:

**Best for:**
- Complex patterns with trends
- Seasonal workloads
- Higher accuracy requirements

**Configuration:**
```bash
# Auto parameters (ARIMA(1,1,1))
--model=arima

# Custom parameters
--model=arima --arima-p=2 --arima-d=1 --arima-q=2
```

See [../../docs/models/arima.md](../../docs/models/arima.md) for details.

## Storage Backends

### In-Memory (Default)

Stores forecasts in process memory.

**Pros:**
- Zero dependencies
- Fast
- Simple

**Cons:**
- No persistence across restarts
- Single instance only
- No HA

**Configuration:**
```bash
--storage=memory
```

### Redis

Stores forecasts in Redis for shared state.

**Pros:**
- Persistence across restarts
- Multiple forecaster instances
- High availability
- TTL-based expiration

**Cons:**
- Redis dependency
- Network latency

**Configuration:**
```bash
--storage=redis \
--redis-addr=redis:6379 \
--redis-password=secret \
--redis-db=0 \
--redis-ttl=1h
```

## HTTP API

The forecaster exposes a REST API on port 8081 (configurable).

### GET /forecast/current

Fetch the latest forecast for a workload.

**Query Parameters:**
- `workload` (required): Workload name

**Response:**
```json
{
  "workload": "my-api",
  "metric": "http_rps",
  "generatedAt": "2025-12-22T10:15:30Z",
  "stepSeconds": 60,
  "horizonSeconds": 1800,
  "values": [420.5, 425.1, 430.2, ...],
  "desiredReplicas": [5, 5, 5, 6, ...]
}
```

**Example:**
```bash
curl "http://localhost:8081/forecast/current?workload=my-api" | jq
```

### GET /metrics

Prometheus metrics endpoint.

**Metrics exposed:**
- `kedastral_predicted_value`: Current predicted value
- `kedastral_desired_replicas`: Desired replica count
- `kedastral_forecast_age_seconds`: Forecast staleness
- `kedastral_adapter_collect_seconds`: Collection latency
- `kedastral_model_predict_seconds`: Prediction latency
- `kedastral_capacity_compute_seconds`: Capacity computation latency
- `kedastral_errors_total`: Error counts

See [../../docs/OBSERVABILITY.md](../../docs/OBSERVABILITY.md) for full metrics reference.

### GET /healthz

Health check endpoint.

**Response:**
- `200 OK`: Healthy
- `503 Service Unavailable`: Unhealthy

**Example:**
```bash
curl http://localhost:8081/healthz
```

## Configuration

### Required Flags

**Single-workload mode:**
```bash
--workload=my-api            # Workload name
--metric=http_rps            # Metric being forecasted
--adapter=prometheus         # Adapter type: prometheus, victoriametrics, or http
```

**Required environment variables for adapters:**
```bash
ADAPTER_QUERY='...'          # Query for prometheus/victoriametrics
ADAPTER_URL='http://...'     # Optional: URL override (has defaults)
```

### Common Flags

```bash
--listen=:8081              # HTTP listen address
--horizon=30m               # Forecast horizon
--step=1m                   # Forecast step size
--interval=30s              # Generation interval
--window=3h                 # Historical data window
--model=baseline            # Model: baseline or arima
--target-per-pod=100        # Target metric per pod
--headroom=1.2              # Safety buffer (1.2 = 20%)
--min=2                     # Minimum replicas
--max=50                    # Maximum replicas
--storage=memory            # Storage: memory or redis
--log-level=info            # Log level
```

See [../../docs/CONFIGURATION.md](../../docs/CONFIGURATION.md) for all options.

## Running Locally

### Development Setup

```bash
# Build
make forecaster

# Run with minimal config
ADAPTER_QUERY='sum(rate(http_requests_total{service="my-api"}[1m]))' \
ADAPTER_URL=http://localhost:9090 \
./bin/forecaster \
  --workload=my-api \
  --metric=http_rps \
  --adapter=prometheus \
  --target-per-pod=100 \
  --log-level=debug
```

### With Redis

```bash
# Start Redis
docker run -d -p 6379:6379 redis:7

# Run forecaster with Redis
ADAPTER_QUERY='sum(rate(http_requests_total{service="my-api"}[1m]))' \
ADAPTER_URL=http://localhost:9090 \
./bin/forecaster \
  --workload=my-api \
  --metric=http_rps \
  --adapter=prometheus \
  --storage=redis \
  --redis-addr=localhost:6379 \
  --target-per-pod=100
```

## Managing Multiple Workloads

The forecaster follows a **one-workload-per-deployment** architecture for security and isolation. To forecast multiple workloads, deploy multiple forecaster instances with different configurations.

See [DEPLOYMENT.md](../../docs/DEPLOYMENT.md#managing-multiple-workloads) for Helm-based multi-deployment patterns.

## Testing

### Check Forecast

```bash
# Fetch current forecast
curl "http://localhost:8081/forecast/current?workload=my-api" | jq

# Check prediction values
curl "http://localhost:8081/forecast/current?workload=my-api" | jq '.values'

# Check desired replicas
curl "http://localhost:8081/forecast/current?workload=my-api" | jq '.desiredReplicas'

# Check forecast age
curl "http://localhost:8081/forecast/current?workload=my-api" | jq '.generatedAt'
```

### Monitor Metrics

```bash
# View all metrics
curl http://localhost:8081/metrics

# Check predicted value
curl -s http://localhost:8081/metrics | grep kedastral_predicted_value

# Check error rate
curl -s http://localhost:8081/metrics | grep kedastral_errors_total
```

### Watch Logs

```bash
# Text format (human-readable)
./bin/forecaster --log-format=text --log-level=debug

# JSON format (machine-readable)
./bin/forecaster --log-format=json --log-level=info | jq
```

## Kubernetes Deployment

### Basic Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kedastral-forecaster
spec:
  replicas: 1
  selector:
    matchLabels:
      component: forecaster
  template:
    metadata:
      labels:
        component: forecaster
    spec:
      containers:
      - name: forecaster
        image: kedastral/forecaster:latest
        env:
          - name: ADAPTER_QUERY
            value: "sum(rate(http_requests_total{service=\"my-api\"}[1m]))"
          - name: ADAPTER_URL
            value: "http://prometheus:9090"
        args:
          - --workload=my-api
          - --metric=http_rps
          - --adapter=prometheus
          - --target-per-pod=100
          - --headroom=1.2
          - --min=2
          - --max=50
        ports:
          - containerPort: 8081
            name: http
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 10
          periodSeconds: 5
        resources:
          requests:
            memory: "256Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"
---
apiVersion: v1
kind: Service
metadata:
  name: kedastral-forecaster
spec:
  selector:
    component: forecaster
  ports:
    - port: 8081
      targetPort: 8081
      name: http
```

### HA Deployment with Redis

See [../../docs/DEPLOYMENT.md](../../docs/DEPLOYMENT.md) for production deployment patterns.

## Troubleshooting

### Problem: No forecasts generated

**Check:**
1. Prometheus connectivity: `kubectl exec -it <pod> -- wget -O- http://prometheus:9090/api/v1/query?query=up`
2. Query returns data: Test query in Prometheus UI
3. Logs: `kubectl logs -l component=forecaster`

### Problem: Predictions are inaccurate

**Solutions:**
1. Increase `--window` for more historical data (e.g., `--window=6h`)
2. Try ARIMA model: `--model=arima`
3. Tune capacity policy: adjust `--headroom` or use `--quantile-level=p90`
4. Check if workload has predictable patterns

See [../../docs/planner/tuning.md](../../docs/planner/tuning.md) for tuning guidance.

### Problem: High memory usage

**Causes:**
- Large window size
- Many workloads
- ARIMA model (5MB per workload vs 1MB for baseline)

**Solutions:**
- Reduce `--window` if possible
- Use baseline model instead of ARIMA
- Increase memory limits in Kubernetes

### Problem: Slow predictions

**Check metrics:**
```bash
curl -s http://localhost:8081/metrics | grep kedastral_model_predict_seconds
```

**Solutions:**
- Use baseline model (faster than ARIMA)
- Reduce horizon/window
- Increase CPU limits

## Development

### Build

```bash
make forecaster
# Binary: bin/forecaster
```

### Test

```bash
# Run forecaster tests
go test ./cmd/forecaster/...

# Run with coverage
go test -cover ./cmd/forecaster/...
```

### Debug

```bash
# Enable debug logging
./bin/forecaster --log-level=debug --log-format=text

# Add trace logging (if implemented)
KEDASTRAL_TRACE=1 ./bin/forecaster --log-level=debug
```

## Next Steps

- **Configuration**: See [../../docs/CONFIGURATION.md](../../docs/CONFIGURATION.md) for all options
- **Models**: See [../../docs/models/](../../docs/models/) for model documentation
- **Capacity Planning**: See [../../docs/planner/](../../docs/planner/) for tuning guidance
- **Deployment**: See [../../docs/DEPLOYMENT.md](../../docs/DEPLOYMENT.md) for production setup
- **Observability**: See [../../docs/OBSERVABILITY.md](../../docs/OBSERVABILITY.md) for metrics reference
