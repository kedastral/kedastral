# Kedastral Test Environment

A complete test environment for experimenting with Kedastral predictive autoscaling on minikube.

## What's Included

### Sample Application
- **Backend API**: Go HTTP server with Prometheus metrics
  - Endpoints: `/`, `/api/task`, `/api/heavy`, `/health`, `/metrics`
  - Exposes request rate, duration, and active connections
  - PostgreSQL integration for realistic workload

- **Load Generator**: Creates predictable traffic patterns
  - `constant`: Steady 60 RPS baseline
  - `hourly-spike`: Spikes every hour (great for testing predictions)
  - `business-hours`: High load 9am-5pm
  - `sine-wave`: Smooth oscillating pattern
  - `double-peak`: Morning (9am) and afternoon (3pm) peaks

### Infrastructure
- **PostgreSQL**: Database for realistic stateful workload
- **Prometheus**: Metrics collection via kube-prometheus-stack
- **KEDA**: Event-driven autoscaling platform
- **Kedastral**: Predictive scaling (forecaster + scaler)

## Prerequisites

- Docker Desktop for Mac
- minikube
- kubectl
- helm

## Quick Start

### 1. Run Setup Script

```bash
cd examples/simple-app
./setup.sh
```

This will:
1. Start minikube (4 CPUs, 8GB RAM)
2. Install KEDA
3. Install Prometheus
4. Build all Docker images
5. Deploy the test application
6. Deploy Kedastral
7. Configure KEDA ScaledObject

Setup takes ~5-10 minutes depending on your internet connection.

### 2. Watch It Work

```bash
# Watch pods scale in real-time
kubectl get pods -n simple-app -w

# In another terminal, watch the HPA
watch kubectl get hpa -n simple-app

# In another terminal, follow forecaster logs
kubectl logs -f -l app=kedastral-forecaster -n simple-app
```

### 3. Experiment with Different Load Patterns

Change the load pattern to see how Kedastral adapts:

```bash
# Try sine wave pattern
kubectl set env deployment/load-generator PATTERN=sine-wave -n simple-app

# Or double peak
kubectl set env deployment/load-generator PATTERN=double-peak -n simple-app

# Or business hours
kubectl set env deployment/load-generator PATTERN=business-hours -n simple-app
```

## Understanding the Setup

### Kedastral Configuration

The forecaster is configured with:
- **PromQL Query**: `sum(rate(api_requests_total{namespace="simple-app"}[1m]))`
- **Target per pod**: 50 RPS
- **Headroom**: 1.2 (20% buffer)
- **Lead time**: 5 minutes (scales up 5 min before needed)
- **Forecast horizon**: 30 minutes
- **Model**: Baseline (EMA + hour-of-day seasonality)

### How It Works

1. **Prometheus** scrapes metrics from the test app every 15s
2. **Kedastral Forecaster** queries Prometheus every 30s:
   - Fetches last 30m of request rate data
   - Runs forecasting model
   - Predicts next 30 minutes of load
   - Calculates desired replicas for each minute
   - Stores forecast snapshot
3. **Kedastral Scaler** receives requests from KEDA:
   - Fetches latest forecast
   - Returns predicted replicas (5 minutes ahead)
4. **KEDA** polls scaler every 15s and updates HPA
5. **HPA** scales the deployment

## Useful Commands

### Monitoring

```bash
# Access Prometheus UI
kubectl port-forward -n monitoring svc/prometheus-operated 9090:9090
# Open http://localhost:9090

# Access Grafana UI
kubectl port-forward -n monitoring svc/prometheus-grafana 3000:80
# Open http://localhost:3000 (user: admin, password: prom-operator)

# Check current forecast
kubectl exec -n simple-app deploy/kedastral-forecaster -- \
  wget -qO- http://localhost:8081/forecast/current?workload=simple-app | jq .
```

### Debugging

```bash
# View all pods
kubectl get pods -n simple-app

# Check forecaster logs
kubectl logs -l app=kedastral-forecaster -n simple-app

# Check scaler logs
kubectl logs -l app=kedastral-scaler -n simple-app

# Check load generator logs
kubectl logs -l app=load-generator -n simple-app

# Describe ScaledObject
kubectl describe scaledobject simple-app-scaledobject -n simple-app

# Check HPA metrics
kubectl get hpa -n simple-app
kubectl describe hpa keda-hpa-simple-app-scaledobject -n simple-app
```

### Testing

```bash
# Test the app directly
kubectl port-forward -n simple-app svc/simple-app 8080:8080
curl http://localhost:8080
curl http://localhost:8080/api/task
curl http://localhost:8080/metrics
```

## Prometheus Queries to Try

Open Prometheus UI and try these queries:

```promql
# Request rate (what Kedastral uses)
sum(rate(api_requests_total{namespace="simple-app"}[1m]))

# Current replicas
count(kube_pod_info{namespace="simple-app", pod=~"simple-app-.*"})

# Predicted replicas (from Kedastral)
kedastral_desired_replicas{workload="simple-app"}

# Forecast age (staleness)
kedastral_forecast_age_seconds{workload="simple-app"}
```

## Grafana Dashboard

Import the included dashboard to visualize:
- Actual vs predicted load
- Current vs desired replicas
- Forecast accuracy
- Scaling events

```bash
# Import monitoring/grafana-dashboard.json in Grafana UI
```

## Experimenting with Models

### Switch to ARIMA Model

Edit `k8s/05-kedastral-forecaster.yaml` and change:

```yaml
- -model=arima
- -arima-p=1
- -arima-d=1
- -arima-q=1
```

Then apply:
```bash
kubectl apply -f k8s/05-kedastral-forecaster.yaml
kubectl rollout restart deployment/kedastral-forecaster -n simple-app
```

### Adjust Lead Time

Longer lead time = more advance notice = smoother scaling:

```yaml
# In forecaster
- -lead-time=10m  # Predict 10 minutes ahead

# In scaler
- -lead-time=10m  # Must match forecaster
```

### Tune Capacity Planning

```yaml
# More conservative (scale up earlier)
- -headroom=1.5  # 50% buffer
- -target-per-pod=40  # Lower threshold

# More aggressive (pack pods tighter)
- -headroom=1.1  # 10% buffer
- -target-per-pod=100  # Higher threshold
```

## Architecture

```
┌──────────────┐
│ Load         │
│ Generator    │──┐
└──────────────┘  │
                  ▼
              ┌────────────┐
              │  Test App  │◄──────┐
              │  (scaled)  │       │
              └────────────┘       │
                    │              │
                    │ metrics      │ scale
                    ▼              │
              ┌────────────┐       │
              │ Prometheus │       │
              └────────────┘       │
                    │              │
                    │ scrape       │
                    ▼              │
         ┌──────────────────┐     │
         │   Kedastral      │     │
         │   Forecaster     │     │
         └──────────────────┘     │
                    │              │
                    │ forecast     │
                    ▼              │
         ┌──────────────────┐     │
         │   Kedastral      │     │
         │   Scaler         │     │
         └──────────────────┘     │
                    │              │
                    │ gRPC         │
                    ▼              │
              ┌────────────┐       │
              │    KEDA    │───────┘
              └────────────┘
                    │
                    ▼
              ┌────────────┐
              │    HPA     │
              └────────────┘
```

## Troubleshooting

### Pods not scaling
- Check KEDA logs: `kubectl logs -n keda-system -l app=keda-operator`
- Check ScaledObject: `kubectl describe scaledobject -n simple-app`
- Verify scaler is reachable: `kubectl exec -n simple-app deploy/kedastral-scaler -- netstat -ln | grep 50051`

### Forecast is stale
- Check forecaster logs for Prometheus connection errors
- Verify Prometheus is running: `kubectl get pods -n monitoring`
- Check query returns data:
  ```bash
  kubectl port-forward -n monitoring svc/prometheus-operated 9090:9090
  # Run query: sum(rate(api_requests_total{namespace="simple-app"}[1m]))
  ```

### Images not found
- Ensure you ran `eval $(minikube docker-env)` before building
- Rebuild images inside minikube context
- Verify: `minikube ssh "docker images | grep kedastral"`

## Cleanup

```bash
./cleanup.sh
```

This will:
1. Delete simple-app namespace (removes all resources)
2. Optionally delete KEDA
3. Optionally delete Prometheus
4. Optionally stop minikube

## Next Steps

Once you understand the basics:

1. **Try different models**: Compare baseline vs ARIMA performance
2. **Tune parameters**: Experiment with lead time, headroom, target per pod
3. **Custom patterns**: Edit load-generator to create your own patterns
4. **Multi-metric**: Add CPU/memory metrics alongside RPS
5. **Production setup**: Try Redis storage for HA forecaster deployment

## Resources

- [Kedastral Documentation](../../README.md)
- [KEDA Documentation](https://keda.sh/docs/)
- [Prometheus Query Basics](https://prometheus.io/docs/prometheus/latest/querying/basics/)
