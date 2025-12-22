# Scaler

The Kedastral Scaler is a KEDA External Scaler that bridges forecasts to Kubernetes autoscaling by implementing the KEDA gRPC protocol.

## What It Does

The scaler:

1. **Implements** the [KEDA External Scaler gRPC API](https://keda.sh/docs/latest/concepts/external-scalers/)
2. **Fetches** forecasts from the forecaster via HTTP
3. **Applies** lead-time logic to select appropriate replica counts
4. **Returns** desired replicas to KEDA via gRPC
5. **Exposes** metrics and health endpoints for monitoring

## Architecture

```
┌──────────────┐
│  Forecaster  │
│   HTTP API   │
└──────┬───────┘
       │ GET /forecast/current
       ▼
┌─────────────────────────────┐
│      Scaler (gRPC)          │
│                             │
│  1. Fetch forecast          │
│  2. Apply lead-time logic   │
│  3. Return replicas         │
└──────────┬──────────────────┘
           │ gRPC
           ▼
    ┌──────────────┐
    │     KEDA     │
    │  (triggers   │
    │   scaling)   │
    └──────┬───────┘
           │
           ▼
    ┌──────────────┐
    │     HPA      │
    │  (scales     │
    │   workload)  │
    └──────────────┘
```

## Key Concepts

### KEDA External Scaler

KEDA External Scalers allow custom metrics to drive autoscaling. The scaler implements three gRPC methods:

1. **IsActive**: Tells KEDA if the scaler has valid data
2. **GetMetricSpec**: Defines the metric specification for KEDA
3. **GetMetrics**: Returns the current metric value (desired replicas)

See [KEDA External Scaler docs](https://keda.sh/docs/latest/concepts/external-scalers/) for protocol details.

### Lead-Time Logic

The scaler implements **proactive scaling** by looking ahead in the forecast:

```
Forecast: [10, 12, 15, 20, 18, 16, 14, 12, ...]
           ^               ^
           t=0             t=lead-time

With 5-minute lead time and 1-minute steps:
- Lead steps = 5
- Look at replicas[0:5] = [10, 12, 15, 20, 18]
- Return MAX = 20 replicas
```

**Why MAX?**
- Ensures capacity is ready **before** demand arrives
- Prevents scaling lag during rapid increases
- Provides buffer for prediction uncertainty

**Scale-down** is gradual due to capacity planner clamps in the forecaster.

### Lead-Time Tuning

| Lead Time | Use Case | Behavior |
|-----------|----------|----------|
| `3m-5m` | Gradual changes, light containers | Moderate proactivity |
| `10m-15m` | Predictable spikes, batch jobs | High proactivity (recommended) |
| `20m+` | Very slow scaling (heavy images) | Maximum proactivity |

**Guidelines:**
- Too short: May not scale up in time
- Too long: May over-provision for distant predictions
- Typical: 10-15 minutes for most workloads

## gRPC Interface

The scaler implements the KEDA External Scaler protocol:

### IsActive

Checks if the scaler has valid forecast data.

**Returns:**
- `true`: Forecast exists and is fresh
- `false`: No forecast or forecast is stale

**Used by KEDA to:**
- Determine if ScaledObject should be active
- Decide when to scale to zero (if configured)

### GetMetricSpec

Defines the metric specification for KEDA.

**Returns:**
```protobuf
{
  metricName: "desired_replicas"
  targetSize: 1
  metricType: Value
}
```

**Interpretation:**
- The metric value returned by `GetMetrics` **is** the desired replica count
- KEDA will scale the workload to match this value

### GetMetrics

Returns the current desired replica count based on forecast + lead-time.

**Returns:**
```protobuf
{
  metricName: "desired_replicas"
  metricValue: 20  // MAX over lead-time window
}
```

**Algorithm:**
1. Fetch forecast from forecaster
2. Calculate lead steps: `leadTime / stepSeconds`
3. Take MAX over `desiredReplicas[0:leadSteps]`
4. Return the maximum to KEDA

## HTTP API

The scaler exposes HTTP endpoints on port 8082 (configurable) for observability.

### GET /metrics

Prometheus metrics endpoint.

**Metrics exposed:**
- `kedastral_scaler_desired_replicas_returned`: Last replica count returned to KEDA
- `kedastral_scaler_forecast_age_seen_seconds`: Forecast age at fetch time
- `kedastral_scaler_forecast_fetch_duration_seconds`: Fetch latency
- `kedastral_scaler_grpc_request_duration_seconds`: gRPC request duration
- `kedastral_scaler_forecast_fetch_errors_total`: Fetch error counts

See [../../docs/OBSERVABILITY.md](../../docs/OBSERVABILITY.md) for full metrics reference.

### GET /healthz

Health check endpoint.

**Response:**
- `200 OK`: Healthy (gRPC server running, forecaster reachable)
- `503 Service Unavailable`: Unhealthy

**Example:**
```bash
curl http://localhost:8082/healthz
```

## Configuration

### Required Flags

```bash
--forecaster-url=http://kedastral-forecaster:8081  # Forecaster HTTP endpoint
```

### Common Flags

```bash
--listen=:50051          # gRPC listen address
--lead-time=10m          # Lookahead window (recommended: 10-15m)
--log-level=info         # Log level
--log-format=text        # Log format: text or json
```

See [../../docs/CONFIGURATION.md](../../docs/CONFIGURATION.md) for all options.

## Running Locally

### Development Setup

```bash
# Build
make scaler

# Run with minimal config
./bin/scaler \
  --forecaster-url=http://localhost:8081 \
  --lead-time=5m \
  --log-level=debug
```

### Test gRPC Interface

Use `grpcurl` to test the gRPC API:

```bash
# Install grpcurl
brew install grpcurl  # macOS
# or: go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest

# List services
grpcurl -plaintext localhost:50051 list

# Call IsActive
grpcurl -plaintext -d '{"scaledObjectRef":{"name":"my-api"}, "metricName":"desired_replicas"}' \
  localhost:50051 externalscaler.ExternalScaler/IsActive

# Call GetMetricSpec
grpcurl -plaintext -d '{"scaledObjectRef":{"name":"my-api"}}' \
  localhost:50051 externalscaler.ExternalScaler/GetMetricSpec

# Call GetMetrics (requires workload query param)
grpcurl -plaintext -d '{"scaledObjectRef":{"name":"my-api"}, "metricName":"desired_replicas"}' \
  localhost:50051 externalscaler.ExternalScaler/GetMetrics
```

### Watch Logs

```bash
# Text format (human-readable)
./bin/scaler --log-format=text --log-level=debug

# JSON format (machine-readable)
./bin/scaler --log-format=json --log-level=info | jq
```

## KEDA Integration

### ScaledObject Configuration

Connect KEDA to the scaler:

```yaml
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: my-api-scaledobject
  namespace: default
spec:
  scaleTargetRef:
    name: my-api              # Target deployment
    kind: Deployment
  pollingInterval: 30         # How often KEDA polls (seconds)
  cooldownPeriod: 300         # Cooldown after scale-down (seconds)
  minReplicaCount: 2          # Minimum replicas (overrides forecaster min)
  maxReplicaCount: 50         # Maximum replicas (overrides forecaster max)
  triggers:
    - type: external
      metadata:
        scalerAddress: kedastral-scaler.default.svc.cluster.local:50051
        workload: my-api      # Must match forecaster workload name
```

**Important:**
- `workload` metadata must match the workload name in forecaster configuration
- `minReplicaCount`/`maxReplicaCount` in ScaledObject override forecaster values
- `pollingInterval` determines how often KEDA fetches metrics (30s typical)

### Verify KEDA Integration

```bash
# Check ScaledObject status
kubectl get scaledobject my-api-scaledobject

# Describe ScaledObject
kubectl describe scaledobject my-api-scaledobject

# Check HPA created by KEDA
kubectl get hpa

# Describe HPA
kubectl describe hpa keda-hpa-my-api-scaledobject

# Check current replicas
kubectl get deployment my-api
```

## Kubernetes Deployment

### Basic Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kedastral-scaler
spec:
  replicas: 1
  selector:
    matchLabels:
      component: scaler
  template:
    metadata:
      labels:
        component: scaler
    spec:
      containers:
      - name: scaler
        image: kedastral/scaler:latest
        args:
          - --forecaster-url=http://kedastral-forecaster:8081
          - --lead-time=10m
        ports:
          - containerPort: 50051
            name: grpc
          - containerPort: 8082
            name: http
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8082
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /healthz
            port: 8082
          initialDelaySeconds: 10
          periodSeconds: 5
        resources:
          requests:
            memory: "64Mi"
            cpu: "50m"
          limits:
            memory: "128Mi"
            cpu: "200m"
---
apiVersion: v1
kind: Service
metadata:
  name: kedastral-scaler
spec:
  selector:
    component: scaler
  ports:
    - port: 50051
      targetPort: 50051
      name: grpc
    - port: 8082
      targetPort: 8082
      name: http
```

### HA Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kedastral-scaler
spec:
  replicas: 2  # Multiple replicas for HA
  strategy:
    type: RollingUpdate
  template:
    spec:
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
            - weight: 100
              podAffinityTerm:
                labelSelector:
                  matchLabels:
                    component: scaler
                topologyKey: kubernetes.io/hostname
      containers:
      - name: scaler
        # ... same as above
```

**Note:** KEDA can load-balance across multiple scaler pods automatically.

## Scaling Behavior

### Proactive Scale-Up

The scaler looks ahead in the forecast to pre-scale before demand:

**Example:**
```
Current replicas: 10
Forecast (next 15 minutes): [10, 12, 15, 20, 18, 16, ...]
Lead time: 10 minutes (10 steps)
Scaler returns: MAX(10, 12, 15, 20, 18, 16) = 20
KEDA scales to: 20 replicas immediately
```

### Gradual Scale-Down

Scale-down is controlled by forecaster's capacity planner:
- `DownMaxPercentPerStep` parameter (default 50%)
- Example: 100 → 50 → 25 → 13 → ...

**Why gradual?**
- Prevents aggressive scale-down
- Provides stability during fluctuations
- Reduces thrashing

### Hybrid Scaling

KEDA takes MAX across all triggers:

```yaml
triggers:
  - type: external       # Kedastral predictive
    metadata:
      scalerAddress: kedastral-scaler:50051
      workload: my-api
  - type: prometheus     # Reactive CPU/RPS
    metadata:
      serverAddress: http://prometheus:9090
      query: sum(rate(http_requests_total[1m]))
      threshold: "1000"
```

**Behavior:**
```
effectiveReplicas = MAX(predictive, reactive)
```

**Safety:**
- If prediction is wrong, reactive trigger catches it
- Best of both worlds: proactive + reactive

## Troubleshooting

### Problem: KEDA not scaling workload

**Check:**
1. ScaledObject status: `kubectl describe scaledobject my-api-scaledobject`
2. HPA status: `kubectl describe hpa keda-hpa-my-api-scaledobject`
3. Scaler logs: `kubectl logs -l component=scaler`
4. Test gRPC: `grpcurl -plaintext <scaler-ip>:50051 externalscaler.ExternalScaler/GetMetrics`

### Problem: Scaler cannot reach forecaster

**Check:**
1. Forecaster service: `kubectl get svc kedastral-forecaster`
2. Network connectivity: `kubectl exec -it <scaler-pod> -- wget -O- http://kedastral-forecaster:8081/healthz`
3. Logs: `kubectl logs -l component=scaler | grep "fetch error"`

### Problem: Stale forecasts

**Check metrics:**
```bash
curl -s http://<scaler>:8082/metrics | grep kedastral_scaler_forecast_age_seen_seconds
```

**Solutions:**
- Check forecaster is running and generating forecasts
- Verify forecaster interval is appropriate (e.g., `--interval=30s`)
- Check for forecaster errors: `kubectl logs -l component=forecaster`

### Problem: Over-scaling or under-scaling

**Tune lead-time:**
- **Over-scaling**: Reduce `--lead-time` (e.g., from 15m to 5m)
- **Under-scaling**: Increase `--lead-time` (e.g., from 5m to 15m)

**Tune capacity policy in forecaster:**
- Adjust `--headroom` or `--quantile-level`
- See [../../docs/planner/tuning.md](../../docs/planner/tuning.md)

## Monitoring

### Check Current State

```bash
# Fetch metrics
curl http://localhost:8082/metrics

# Check last returned replicas
curl -s http://localhost:8082/metrics | grep kedastral_scaler_desired_replicas_returned

# Check forecast age
curl -s http://localhost:8082/metrics | grep kedastral_scaler_forecast_age_seen_seconds

# Check fetch errors
curl -s http://localhost:8082/metrics | grep kedastral_scaler_forecast_fetch_errors_total
```

### Watch Scaling Events

```bash
# Watch HPA events
kubectl get hpa -w

# Watch deployment scaling
kubectl get deployment my-api -w

# Watch scaler logs
kubectl logs -l component=scaler -f

# Watch KEDA operator logs
kubectl logs -n keda -l app=keda-operator -f
```

## Development

### Build

```bash
make scaler
# Binary: bin/scaler
```

### Test

```bash
# Run scaler tests
go test ./cmd/scaler/...

# Run with coverage
go test -cover ./cmd/scaler/...
```

### Debug

```bash
# Enable debug logging
./bin/scaler --log-level=debug --log-format=text

# Test with curl (for HTTP endpoints)
curl http://localhost:8082/healthz
curl http://localhost:8082/metrics

# Test with grpcurl (for gRPC)
grpcurl -plaintext localhost:50051 list
grpcurl -plaintext -d '{"scaledObjectRef":{"name":"my-api"}}' \
  localhost:50051 externalscaler.ExternalScaler/GetMetrics
```

## Next Steps

- **Configuration**: See [../../docs/CONFIGURATION.md](../../docs/CONFIGURATION.md) for all options
- **KEDA Integration**: See [examples/scaled-object.yaml](../../examples/scaled-object.yaml) for examples
- **Tuning**: See [../../docs/planner/tuning.md](../../docs/planner/tuning.md) for capacity optimization
- **Deployment**: See [../../docs/DEPLOYMENT.md](../../docs/DEPLOYMENT.md) for production setup
- **Observability**: See [../../docs/OBSERVABILITY.md](../../docs/OBSERVABILITY.md) for metrics reference
