# Quick Start Guide

## Setup (5 minutes)

```bash
cd examples/simple-app
./setup.sh
```

Wait for completion, then verify:

```bash
kubectl get pods -n simple-app
```

You should see:
- `simple-app-*` pods (2 initially, will scale)
- `kedastral-forecaster-*`
- `kedastral-scaler-*`
- `load-generator-*`
- `postgres-*`

## Watch Predictive Scaling

Open 3 terminal windows:

### Terminal 1: Watch Pods Scale
```bash
kubectl get pods -n simple-app -w
```

### Terminal 2: Watch Forecaster
```bash
kubectl logs -f -l app=kedastral-forecaster -n simple-app
```

### Terminal 3: Watch HPA
```bash
watch kubectl get hpa -n simple-app
```

## Experiment

### Change Load Pattern

```bash
# Hourly spikes (default)
kubectl set env deployment/load-generator PATTERN=hourly-spike -n simple-app

# Smooth sine wave
kubectl set env deployment/load-generator PATTERN=sine-wave -n simple-app

# Business hours (9-5)
kubectl set env deployment/load-generator PATTERN=business-hours -n simple-app

# Double peak (9am & 3pm)
kubectl set env deployment/load-generator PATTERN=double-peak -n simple-app

# Constant baseline
kubectl set env deployment/load-generator PATTERN=constant -n simple-app
```

### Check Current Forecast

```bash
kubectl exec -n simple-app deploy/kedastral-forecaster -- \
  wget -qO- http://localhost:8081/forecast/current?workload=simple-app | jq .
```

### Switch to ARIMA Model

```bash
# Edit forecaster deployment
kubectl edit deployment kedastral-forecaster -n simple-app

# Change:
#   - -model=baseline
# To:
#   - -model=arima
#   - -arima-p=1
#   - -arima-d=1
#   - -arima-q=1
```

## Access UIs

### Prometheus
```bash
kubectl port-forward -n monitoring svc/prometheus-operated 9090:9090
```
Open http://localhost:9090

Try query: `sum(rate(api_requests_total{namespace="simple-app"}[1m]))`

### Grafana
```bash
kubectl port-forward -n monitoring svc/prometheus-grafana 3000:80
```
Open http://localhost:3000
- User: `admin`
- Password: `prom-operator`

Import dashboard: `monitoring/grafana-dashboard.json`

## Cleanup

```bash
./cleanup.sh
```

## Troubleshooting

### No scaling happening?
```bash
# Check KEDA
kubectl describe scaledobject simple-app-scaledobject -n simple-app

# Check scaler is responding
kubectl logs -l app=kedastral-scaler -n simple-app | tail -20

# Check forecast age (should be < 60s)
kubectl logs -l app=kedastral-forecaster -n simple-app | grep "forecast_age"
```

### Forecast is stale?
```bash
# Check Prometheus connectivity
kubectl logs -l app=kedastral-forecaster -n simple-app | grep -i error

# Verify Prometheus has data
kubectl port-forward -n monitoring svc/prometheus-operated 9090:9090
# Open UI and run: api_requests_total{namespace="simple-app"}
```

## What to Expect

With `hourly-spike` pattern:
- Load spikes to ~200 RPS at :00 minutes
- Kedastral predicts the spike 5 minutes early (:55)
- Pods start scaling up before the load arrives
- Compare this to reactive scaling (disable Kedastral) to see the difference

With `sine-wave` pattern:
- Smooth oscillation 20-140 RPS
- Watch how Kedastral tracks the pattern
- Replicas smoothly adjust ahead of the curve

**Key insight**: Pods should start scaling UP before you see RPS increase, and scale DOWN before RPS drops. That's predictive scaling!
