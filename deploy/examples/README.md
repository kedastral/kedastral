# Kedastral Examples

This directory contains example configurations for deploying and using Kedastral.

## Files

### Helm Values

- **values-example.yaml** - Example Helm values for a typical production deployment
  - Demonstrates configuration for an HTTP API with predictive autoscaling
  - Uses Redis for forecast storage
  - Includes sensible defaults for capacity planning

### KEDA ScaledObjects

- **scaledobject-basic.yaml** - Basic ScaledObject using only Kedastral's predictive scaler
  - Simplest configuration for pure predictive scaling
  - Recommended for workloads with stable, predictable patterns

- **scaledobject-multi-trigger.yaml** - Hybrid approach combining predictive and reactive scaling
  - Kedastral provides proactive scaling based on forecasts
  - Prometheus trigger acts as a safety net for unexpected load spikes
  - KEDA uses MAX across all triggers for optimal protection

## Quick Start

### 1. Install Kedastral with Helm

```bash
# Install with example values
helm install kedastral ./deploy/helm/kedastral \
  -f deploy/examples/values-example.yaml \
  --set forecaster.config.workload=your-deployment-name \
  --set forecaster.config.promQuery='sum(rate(your_metric[1m]))'
```

### 2. Create a ScaledObject

```bash
# Basic predictive scaling
kubectl apply -f deploy/examples/scaledobject-basic.yaml

# Or hybrid predictive + reactive scaling
kubectl apply -f deploy/examples/scaledobject-multi-trigger.yaml
```

### 3. Verify Installation

```bash
# Check Kedastral pods
kubectl get pods -l app.kubernetes.io/name=kedastral

# View forecaster logs
kubectl logs -l app.kubernetes.io/component=forecaster -f

# Check forecast API
kubectl port-forward svc/kedastral-forecaster 8081:8081
curl http://localhost:8081/forecast/current?workload=my-api
```

## Configuration Guide

### Workload Identification

The `workload` field must match across:
1. Forecaster config: `forecaster.config.workload`
2. ScaledObject metadata: `scalerAddress` metadata
3. Target deployment: `scaleTargetRef.name`

### Capacity Planning Parameters

- **targetPerPod**: Metric value that a single pod can handle comfortably
- **headroom**: Safety buffer (1.2 = 20% extra capacity)
- **minReplicas/maxReplicas**: Hard bounds (should match ScaledObject)
- **upMaxFactor**: Maximum scale-up multiplier per forecast step
- **downMaxPercent**: Maximum scale-down percentage per forecast step

### Timing Parameters

- **horizon**: How far ahead to predict (recommended: 15-60m)
- **step**: Forecast granularity (recommended: 1-5m)
- **interval**: How often to update forecasts (recommended: 30s-2m)
- **window**: Historical data for training (recommended: 1-24h depending on patterns)
- **leadTime** (scaler): How far ahead to look for proactive scaling (recommended: 5-15m)

### Storage Selection

**Memory** (default):
- Pros: Simple, no dependencies
- Cons: Forecasts lost on pod restart
- Use for: Development, testing

**Redis**:
- Pros: Persistent, can share across forecaster replicas
- Cons: Requires Redis deployment
- Use for: Production

## Prometheus Query Examples

```yaml
# HTTP requests per second
promQuery: 'sum(rate(http_requests_total{deployment="my-api"}[1m]))'

# Queue depth
promQuery: 'sum(rabbitmq_queue_messages{queue="my-queue"})'

# CPU usage (millicores)
promQuery: 'sum(rate(container_cpu_usage_seconds_total{pod=~"my-api-.*"}[1m])) * 1000'

# Custom business metric
promQuery: 'sum(rate(orders_processed_total[1m]))'
```

## Troubleshooting

### Forecaster not generating forecasts

```bash
# Check forecaster logs
kubectl logs -l app.kubernetes.io/component=forecaster

# Verify Prometheus connectivity
kubectl exec -it deployment/kedastral-forecaster -- wget -O- http://prometheus:9090/api/v1/query?query=up

# Check configuration
kubectl get deployment kedastral-forecaster -o yaml | grep -A 20 env:
```

### Scaler not returning replicas to KEDA

```bash
# Check scaler logs
kubectl logs -l app.kubernetes.io/component=scaler

# Test scaler gRPC health
kubectl exec -it deployment/kedastral-scaler -- wget -O- http://localhost:8082/healthz

# Verify ScaledObject configuration
kubectl get scaledobject -o yaml
```

### Scaling too aggressive or conservative

Adjust these parameters in order:
1. **leadTime** - How far ahead the scaler looks
2. **headroom** - Safety buffer multiplier
3. **upMaxFactor/downMaxPercent** - Rate limiting
4. **horizon** - How far ahead to predict

## Advanced Topics

### Multi-Workload Deployment

Deploy separate forecaster instances per workload:

```bash
# Workload 1
helm install kedastral-api1 ./deploy/helm/kedastral \
  --set forecaster.config.workload=api1 \
  --set forecaster.config.promQuery='...'

# Workload 2
helm install kedastral-api2 ./deploy/helm/kedastral \
  --set forecaster.config.workload=api2 \
  --set forecaster.config.promQuery='...'
```

### Custom Forecasting Models

Kedastral supports ARIMA for more sophisticated time series forecasting:

```yaml
forecaster:
  config:
    model: arima
    arima:
      p: 0  # Auto-detect AR order
      d: 0  # Auto-detect differencing
      q: 0  # Auto-detect MA order
```

Set to non-zero values to override auto-detection.
