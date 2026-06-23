# Quick Start Guide

This guide will help you get Kedastral up and running in your Kubernetes cluster in under 10 minutes.

## Prerequisites

- Go 1.26 or later (for building from source)
- Kubernetes cluster (v1.20+)
- KEDA installed ([installation guide](https://keda.sh/docs/latest/deploy/))
- Prometheus running in the cluster

## Building from Source

```bash
# Clone the repository
git clone https://github.com/kedastral/kedastral.git
cd kedastral

# Build both forecaster and scaler
make build

# Or build individually
make forecaster
make scaler

# Run tests
make test
```

### Using Makefile

```bash
make build           # Build both forecaster and scaler
make test            # Run all tests
make test-coverage   # Run tests with coverage report
make clean           # Remove build artifacts
make help            # Show all available targets
```

## Deploy with the Operator (recommended)

The operator is the easiest way to run Kedastral in a cluster. You declare workloads
as `ForecastPolicy` and `DataSource` resources; the forecaster runs the forecast loop
and generates the KEDA `ScaledObject` for each policy automatically.

### 1. Install the chart in operator mode

```bash
helm install kedastral deploy/helm/kedastral \
  --namespace kedastral --create-namespace \
  --set forecaster.operator.enabled=true \
  --set forecaster.operator.scalerAddress=kedastral-scaler:50051
```

This installs the `ForecastPolicy`/`DataSource` CRDs, the forecaster (with the embedded
controller and RBAC), and the scaler.

### 2. Declare a data source and a policy

```bash
kubectl apply -f deploy/examples/datasource.yaml
kubectl apply -f deploy/examples/forecastpolicy.yaml
```

A minimal pair looks like:

```yaml
apiVersion: kedastral.io/v1alpha1
kind: DataSource
metadata:
  name: prometheus
spec:
  type: prometheus
  config:
    url: http://prometheus.monitoring:9090
    query: sum(rate(http_requests_total{app="web-api"}[1m]))
---
apiVersion: kedastral.io/v1alpha1
kind: ForecastPolicy
metadata:
  name: web-api
spec:
  scaleTargetRef:
    name: web-api
  metric: http_rps
  dataSourceRef:
    name: prometheus
  model:
    type: baseline
  capacity:
    targetPerPod: 100.0
    minReplicas: 2
    maxReplicas: 50
  leadTime: 10m
```

### 3. Watch it work

```bash
# The policy reports status (current/desired replicas, last forecast time)
kubectl get forecastpolicies
kubectl describe forecastpolicy web-api

# A ScaledObject is generated and owned by the policy
kubectl get scaledobjects

# KEDA creates the HPA from it
kubectl get hpa
```

Deleting the `ForecastPolicy` stops its forecast loop and garbage-collects the
generated `ScaledObject`. See the [Operator Guide](OPERATOR.md) for the full CRD
reference and reconciliation behavior.

## Running Locally

If you prefer flags/env over CRDs (local development, or non-operator deployments),
run the binaries directly.

### 1. Start the Forecaster

The forecaster generates predictions and exposes them via HTTP:

```bash
ADAPTER_QUERY='sum(rate(http_requests_total{service="my-api"}[1m]))' \
ADAPTER_URL=http://localhost:9090 \
./bin/forecaster \
  -workload=my-api \
  -metric=http_rps \
  -adapter=prometheus \
  -target-per-pod=100 \
  -headroom=1.2 \
  -min=2 \
  -max=50 \
  -log-level=info
```

Check the forecast:
```bash
curl "http://localhost:8081/forecast/current?workload=my-api"
```

### 2. Start the Scaler

The scaler implements the KEDA External Scaler gRPC interface:

```bash
./bin/scaler \
  -forecaster-url=http://localhost:8081 \
  -lead-time=5m \
  -log-level=info
```

The scaler exposes:
- gRPC on `:50051` for KEDA
- HTTP metrics on `:8082`

### 3. Configure KEDA

Apply a ScaledObject to connect KEDA to Kedastral:

```yaml
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: my-api-scaledobject
spec:
  scaleTargetRef:
    name: my-api
    kind: Deployment
  pollingInterval: 30
  minReplicaCount: 2
  maxReplicaCount: 50
  triggers:
    - type: external
      metadata:
        scalerAddress: kedastral-scaler:50051
        workload: my-api
```

## Deploying to Kubernetes

See the [examples/](../examples/) directory for complete Kubernetes deployment manifests:

- **[examples/deployment.yaml](../examples/deployment.yaml)** - Complete deployment for forecaster and scaler
- **[examples/deployment-redis.yaml](../examples/deployment-redis.yaml)** - HA deployment with Redis storage
- **[examples/scaled-object.yaml](../examples/scaled-object.yaml)** - KEDA ScaledObject configuration
- **[examples/README.md](../examples/README.md)** - Detailed usage guide with configuration tables and troubleshooting

### Quick Deploy

```bash
# Deploy Kedastral components
kubectl apply -f examples/deployment.yaml

# Configure KEDA scaling
kubectl apply -f examples/scaled-object.yaml
```

### Monitor Your Deployment

```bash
# Watch forecaster logs
kubectl logs -l component=forecaster -f

# Watch scaler logs
kubectl logs -l component=scaler -f

# Check current forecast
kubectl port-forward svc/kedastral-forecaster 8081:8081
curl "http://localhost:8081/forecast/current?workload=my-api"

# View metrics
kubectl port-forward svc/kedastral-forecaster 8081:8081
curl "http://localhost:8081/metrics"
```

## Model Selection

Kedastral supports multiple forecasting models. Choose the one that best fits your workload:

### Baseline (Default)

Fast statistical model with trend and seasonality detection:

```bash
./bin/forecaster --workload=my-api --model=baseline
```

- **Best for**: Workloads with recurring patterns
- **Training**: None required
- **Startup**: Immediate

See [models/baseline.md](models/baseline.md) for details.

### ARIMA

Time-series forecasting with AutoRegressive Integrated Moving Average:

```bash
# Auto parameters (ARIMA(1,1,1))
./bin/forecaster --workload=my-api --model=arima

# Custom parameters
./bin/forecaster --workload=my-api --model=arima --arima-p=2 --arima-d=1 --arima-q=2
```

- **Best for**: Complex patterns with trends and seasonality
- **Training**: Required (uses historical window)
- **Startup**: Requires warm-up data

See [models/arima.md](models/arima.md) for details.

### SARIMA

Seasonal ARIMA for workloads with a strong daily/weekly cycle:

```bash
./bin/forecaster --workload=my-api --model=sarima --sarima-s=24
```

- **Best for**: Recurring seasonal patterns (hourly-with-daily, daily-with-weekly)
- **Training**: Required (uses historical window)

With the operator, set `model.type: sarima` and `model.sarima.*` on the ForecastPolicy.

## Storage Backends

### In-Memory (Default)

Zero configuration, best for single-instance deployments:

```bash
./bin/forecaster --workload=my-api --storage=memory
```

### Redis (HA Deployments)

Shared storage for multi-instance forecasters:

```bash
./bin/forecaster \
  --storage=redis \
  --redis-addr=redis:6379 \
  --redis-ttl=1h \
  --workload=my-api
```

See [examples/deployment-redis.yaml](../examples/deployment-redis.yaml) for a complete HA setup.

## Verification

Once deployed, verify everything is working:

1. **Check forecaster health:**
   ```bash
   kubectl get pods -l component=forecaster
   curl http://kedastral-forecaster:8081/healthz
   ```

2. **Check scaler health:**
   ```bash
   kubectl get pods -l component=scaler
   curl http://kedastral-scaler:8082/healthz
   ```

3. **Verify KEDA integration:**
   ```bash
   kubectl get scaledobject my-api-scaledobject
   kubectl describe scaledobject my-api-scaledobject
   ```

4. **Monitor scaling events:**
   ```bash
   kubectl get hpa
   kubectl describe hpa keda-hpa-my-api-scaledobject
   ```

## Next Steps

- **Configuration**: See [CONFIGURATION.md](CONFIGURATION.md) for all available options
- **Architecture**: Understand the system design in [ARCHITECTURE.md](ARCHITECTURE.md)
- **Production Deployment**: Review [DEPLOYMENT.md](DEPLOYMENT.md) for production considerations
- **Observability**: Set up monitoring with [OBSERVABILITY.md](OBSERVABILITY.md)
- **Tuning**: Optimize capacity planning in [planner/tuning.md](planner/tuning.md)

## Troubleshooting

See [examples/README.md](../examples/README.md) for common issues and solutions.
