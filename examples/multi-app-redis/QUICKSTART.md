# Quick Start Guide - Multi-App Redis

## Setup (5 minutes)

```bash
cd examples/multi-app-redis
./setup.sh
```

Wait for completion, then verify:

```bash
kubectl get pods -n multi-app-redis
```

You should see:
- `api-app-*` pods (2 initially, will scale)
- `worker-app-*` pods (2 initially, will scale)
- `kedastral-forecaster-api-*`
- `kedastral-forecaster-worker-*`
- `kedastral-scaler-*`
- `api-load-generator-*`
- `worker-load-generator-*`
- `redis-*`

## Watch Predictive Scaling (Both Workloads)

Open 4 terminal windows:

### Terminal 1: Watch Pods Scale
```bash
kubectl get pods -n multi-app-redis -w
```

### Terminal 2: Watch Both HPAs
```bash
watch kubectl get hpa -n multi-app-redis
```

### Terminal 3: Watch API Forecaster
```bash
kubectl logs -f -l app=kedastral-forecaster-api -n multi-app-redis
```

### Terminal 4: Watch Worker Forecaster
```bash
kubectl logs -f -l app=kedastral-forecaster-worker -n multi-app-redis
```

## Observe Different Patterns

- **API**: Hourly spikes at :00 (should pre-scale at :55)
- **Worker**: Business hours pattern (ramps up/down with time)

## Experiment

### Change Load Patterns

```bash
# API load patterns
kubectl set env deployment/api-load-generator PATTERN=sine-wave -n multi-app-redis
kubectl set env deployment/api-load-generator PATTERN=double-peak -n multi-app-redis

# Worker load patterns
kubectl set env deployment/worker-load-generator PATTERN=hourly-spike -n multi-app-redis
kubectl set env deployment/worker-load-generator PATTERN=constant -n multi-app-redis
```

### Check Forecasts (via Redis)

```bash
# View Redis keys
kubectl exec -n multi-app-redis deploy/redis -- redis-cli KEYS '*'

# View API forecast from Redis
kubectl exec -n multi-app-redis deploy/redis -- redis-cli GET forecast:api-app | jq .

# View Worker forecast from Redis
kubectl exec -n multi-app-redis deploy/redis -- redis-cli GET forecast:worker-app | jq .

# Check via forecaster HTTP endpoints
kubectl exec -n multi-app-redis deploy/kedastral-scaler -- \
  wget -qO- http://kedastral-forecaster-api:8081/forecast/current?workload=api-app | jq .

kubectl exec -n multi-app-redis deploy/kedastral-scaler -- \
  wget -qO- http://kedastral-forecaster-worker:8081/forecast/current?workload=worker-app | jq .
```

### Test Redis Shared Storage

```bash
# Scale API forecaster to 2 replicas (both writing to Redis)
kubectl scale deployment/kedastral-forecaster-api --replicas=2 -n multi-app-redis

# Watch both replicas work (last write wins, conflicts unlikely with 30s interval)
kubectl get pods -l app=kedastral-forecaster-api -n multi-app-redis

# Delete Redis pod to test restart
kubectl delete pod -l app=redis -n multi-app-redis

# Watch forecasters repopulate Redis within 30s
kubectl logs -f -l app=kedastral-forecaster-api -n multi-app-redis | grep "stored"
```

### Compare Workloads Side-by-Side

```bash
# Watch just the app pods
watch 'kubectl get pods -n multi-app-redis | grep -E "(api-app|worker-app)" | grep -v generator'

# Compare HPA metrics
watch 'kubectl get hpa -n multi-app-redis -o wide'
```

## Access UIs

### Prometheus
```bash
kubectl port-forward -n monitoring svc/prometheus-operated 9090:9090
```
Open http://localhost:9090

Try queries:
```promql
# API RPS
sum(rate(api_requests_total{namespace="multi-app-redis",app="api-app"}[1m]))

# Worker RPS
sum(rate(api_requests_total{namespace="multi-app-redis",app="worker-app"}[1m]))
```

## Cleanup

```bash
./cleanup.sh
```

## Troubleshooting

### No scaling happening?
```bash
# Check both ScaledObjects
kubectl describe scaledobject api-app-scaledobject -n multi-app-redis
kubectl describe scaledobject worker-app-scaledobject -n multi-app-redis

# Check scaler is responding for both workloads
kubectl logs -l app=kedastral-scaler -n multi-app-redis | tail -20

# Verify forecasts in Redis
kubectl exec -n multi-app-redis deploy/redis -- redis-cli KEYS 'forecast:*'
```

### Forecasts are stale?
```bash
# Check both forecaster logs for errors
kubectl logs -l app=kedastral-forecaster-api -n multi-app-redis | grep -i error
kubectl logs -l app=kedastral-forecaster-worker -n multi-app-redis | grep -i error

# Verify Prometheus connectivity
kubectl port-forward -n monitoring svc/prometheus-operated 9090:9090
# Open UI and verify both queries return data
```

### Redis connection issues?
```bash
# Check Redis is running
kubectl get pods -l app=redis -n multi-app-redis

# Test Redis connectivity
kubectl exec -n multi-app-redis deploy/redis -- redis-cli PING

# Check forecaster can reach Redis
kubectl exec -n multi-app-redis deploy/kedastral-forecaster-api -- \
  sh -c 'nc -zv redis 6379'
```

## What to Expect

### API (hourly-spike pattern):
- Load spikes to ~150 RPS at :00 minutes
- Kedastral predicts the spike 5 minutes early (:55)
- API pods start scaling up before the load arrives
- Scales back down gradually after spike passes

### Worker (business-hours pattern):
- Low load overnight and weekends
- Ramps up starting at 9am
- High load throughout business hours
- Ramps down after 5pm

**Key insight**: Both workloads scale independently based on their own forecasts, but share the same Redis storage and scaler infrastructure. This is a production-ready multi-tenant pattern!
