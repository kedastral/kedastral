# Kedastral Multi-App Redis Example

An advanced example demonstrating Kedastral predictive autoscaling with:
- **Multiple workloads** scaling independently
- **Redis storage** for shared forecast state
- **Production-like architecture** with HA considerations

## What's Different from Simple Example

### Architecture Highlights
- **2 Applications**: API service + Worker service with different load patterns
- **2 Forecasters**: One per workload, both writing to Redis
- **1 Scaler**: Shared scaler reading from Redis for both workloads
- **Redis**: Centralized storage enabling forecaster HA and persistence

### Why Redis?
- **High Availability**: Multiple forecaster replicas can share state
- **Persistence**: Forecasts survive pod restarts
- **Multi-workload**: Single scaler serves multiple applications
- **Production Ready**: External storage is recommended for production

## Prerequisites

- Docker Desktop for Mac
- minikube
- kubectl
- helm

## Quick Start

### 1. Run Setup Script

```bash
cd examples/multi-app-redis
./setup.sh
```

This will:
1. Start minikube (4 CPUs, 8GB RAM)
2. Install KEDA
3. Install Prometheus
4. Deploy Redis
5. Build all Docker images (2 apps + load generators + Kedastral)
6. Deploy both applications
7. Deploy both Kedastral forecasters (writing to Redis)
8. Deploy Kedastral scaler (reading from Redis)
9. Configure 2 KEDA ScaledObjects

Setup takes ~5-10 minutes depending on your internet connection.

### 2. Watch Both Workloads Scale

```bash
# Watch all pods scale in real-time
kubectl get pods -n multi-app-redis -w

# In another terminal, watch both HPAs
watch kubectl get hpa -n multi-app-redis

# In another terminal, follow API forecaster logs
kubectl logs -f -l app=kedastral-forecaster-api -n multi-app-redis

# In yet another terminal, follow Worker forecaster logs
kubectl logs -f -l app=kedastral-forecaster-worker -n multi-app-redis
```

### 3. Observe Different Patterns

The two workloads have different characteristics:

**API App:**
- Pattern: `hourly-spike` - predictable spikes every hour
- Target: 50 RPS per pod
- Headroom: 1.2 (20% buffer)
- Max replicas: 20

**Worker App:**
- Pattern: `business-hours` - high load 9am-5pm
- Target: 40 RPS per pod
- Headroom: 1.3 (30% buffer)
- Max replicas: 15

## Understanding the Setup

### Redis-Based Architecture

```
┌─────────────────┐      ┌─────────────────┐
│  API Forecaster │      │Worker Forecaster│
└────────┬────────┘      └────────┬────────┘
         │                        │
         │    ┌──────────┐       │
         └───►│  Redis   │◄──────┘
              └────┬─────┘
                   │
         ┌─────────▼──────────┐
         │  Kedastral Scaler  │
         └─────────┬──────────┘
                   │
         ┌─────────▼──────────┐
         │       KEDA         │
         └────────────────────┘
```

### Kedastral Configuration

**API Forecaster:**
- PromQL: `sum(rate(api_requests_total{namespace="multi-app-redis",app="api-app"}[1m]))`
- Storage: Redis at `redis:6379`
- Workload: `api-app`

**Worker Forecaster:**
- PromQL: `sum(rate(api_requests_total{namespace="multi-app-redis",app="worker-app"}[1m]))`
- Storage: Redis at `redis:6379`
- Workload: `worker-app`

**Scaler:**
- Storage: Redis at `redis:6379`
- Serves both workloads via single gRPC endpoint
- Lead time: 5 minutes
- Stale threshold: 2 minutes

### How It Works

1. **Prometheus** scrapes metrics from both apps
2. **API Forecaster** queries Prometheus every 30s:
   - Fetches API request rate data
   - Runs forecasting model
   - Stores forecast in Redis under key `forecast:api-app`
3. **Worker Forecaster** queries Prometheus every 30s:
   - Fetches Worker request rate data
   - Runs forecasting model
   - Stores forecast in Redis under key `forecast:worker-app`
4. **Scaler** receives requests from KEDA for both workloads:
   - Reads forecast from Redis based on workload name
   - Returns predicted replicas (5 minutes ahead)
5. **KEDA** manages 2 ScaledObjects, polls scaler every 15s
6. **HPA** scales both deployments independently

## Useful Commands

### Monitoring

```bash
# Access Prometheus UI
kubectl port-forward -n monitoring svc/prometheus-operated 9090:9090
# Open http://localhost:9090

# Check API forecast via scaler
kubectl exec -n multi-app-redis deploy/kedastral-scaler -- \
  wget -qO- http://kedastral-forecaster-api:8081/forecast/current?workload=api-app | jq .

# Check Worker forecast via scaler
kubectl exec -n multi-app-redis deploy/kedastral-scaler -- \
  wget -qO- http://kedastral-forecaster-worker:8081/forecast/current?workload=worker-app | jq .

# Inspect Redis keys
kubectl exec -n multi-app-redis deploy/redis -- redis-cli KEYS '*'

# View raw forecast data in Redis
kubectl exec -n multi-app-redis deploy/redis -- redis-cli GET forecast:api-app
kubectl exec -n multi-app-redis deploy/redis -- redis-cli GET forecast:worker-app

# Check Redis info
kubectl exec -n multi-app-redis deploy/redis -- redis-cli INFO stats
```

### Debugging

```bash
# View all pods
kubectl get pods -n multi-app-redis

# Check API forecaster logs
kubectl logs -l app=kedastral-forecaster-api -n multi-app-redis

# Check Worker forecaster logs
kubectl logs -l app=kedastral-forecaster-worker -n multi-app-redis

# Check scaler logs
kubectl logs -l app=kedastral-scaler -n multi-app-redis

# Describe both ScaledObjects
kubectl describe scaledobject api-app-scaledobject -n multi-app-redis
kubectl describe scaledobject worker-app-scaledobject -n multi-app-redis

# Check both HPAs
kubectl get hpa -n multi-app-redis
kubectl describe hpa keda-hpa-api-app-scaledobject -n multi-app-redis
kubectl describe hpa keda-hpa-worker-app-scaledobject -n multi-app-redis
```

### Testing

```bash
# Test API app directly
kubectl port-forward -n multi-app-redis svc/api-app 8080:8080
curl http://localhost:8080

# Test Worker app directly
kubectl port-forward -n multi-app-redis svc/worker-app 8081:8080
curl http://localhost:8081

# Change load patterns
kubectl set env deployment/api-load-generator PATTERN=sine-wave -n multi-app-redis
kubectl set env deployment/worker-load-generator PATTERN=double-peak -n multi-app-redis
```

## Experimenting

### Scale Forecasters Horizontally

Since forecasters use Redis, you can run multiple replicas:

```bash
# Scale API forecaster to 2 replicas
kubectl scale deployment/kedastral-forecaster-api --replicas=2 -n multi-app-redis

# Both will write to Redis; last write wins (30s interval makes conflicts unlikely)
```

### Simulate Forecaster Restart

```bash
# Delete forecaster pod
kubectl delete pod -l app=kedastral-forecaster-api -n multi-app-redis

# Watch scaler continue to serve from Redis while forecaster restarts
kubectl logs -f -l app=kedastral-scaler -n multi-app-redis
```

### Compare Workload Behaviors

```bash
# API has hourly spikes - watch it scale up predictively before :00
# Worker has business hours pattern - watch it ramp up/down with time of day

# Compare scaling behavior side-by-side
watch 'kubectl get pods -n multi-app-redis | grep -E "(api-app|worker-app)"'
```

### Test Redis Persistence

```bash
# Delete Redis pod (data lost - restart scenario)
kubectl delete pod -l app=redis -n multi-app-redis

# Watch forecasters repopulate Redis within 30s
kubectl logs -f -l app=kedastral-forecaster-api -n multi-app-redis | grep "stored forecast"
```

## Prometheus Queries to Try

```promql
# API request rate
sum(rate(api_requests_total{namespace="multi-app-redis",app="api-app"}[1m]))

# Worker request rate
sum(rate(api_requests_total{namespace="multi-app-redis",app="worker-app"}[1m]))

# API replica count
count(kube_pod_info{namespace="multi-app-redis", pod=~"api-app-.*"})

# Worker replica count
count(kube_pod_info{namespace="multi-app-redis", pod=~"worker-app-.*"})

# Predicted replicas (if forecaster exports metrics)
kedastral_desired_replicas{workload="api-app"}
kedastral_desired_replicas{workload="worker-app"}
```

## Production Considerations

This example demonstrates production-ready patterns:

### ✅ Redis Storage
- Enables forecaster HA via multiple replicas
- Provides persistence across pod restarts
- Allows single scaler to serve multiple workloads

### ✅ Separate Forecasters
- Independent configuration per workload
- Different PromQL queries and capacity policies
- Can scale/restart independently

### ✅ Shared Scaler
- Efficient resource usage
- Simplified RBAC (single service account)
- Centralized scaling logic

### 🔧 For Production, Also Consider:
- **Redis HA**: Use Redis Sentinel or Redis Cluster
- **Persistent Volumes**: Add PV for Redis data
- **Resource Limits**: Tune forecaster/scaler resources
- **Metrics Export**: Enable Prometheus metrics from forecasters
- **Alerts**: Set up alerts for stale forecasts
- **TLS**: Enable TLS for Redis connections
- **Authentication**: Use Redis AUTH

## Troubleshooting

### Forecasts not appearing in Redis
```bash
# Check forecaster logs for errors
kubectl logs -l app=kedastral-forecaster-api -n multi-app-redis | grep -i error

# Verify Redis connectivity
kubectl exec -n multi-app-redis deploy/kedastral-forecaster-api -- \
  wget -qO- http://localhost:8081/healthz

# Check if Redis is accepting connections
kubectl exec -n multi-app-redis deploy/redis -- redis-cli PING
```

### Scaler returning stale forecasts
```bash
# Check when forecast was last updated
kubectl exec -n multi-app-redis deploy/redis -- \
  redis-cli GET forecast:api-app | jq .generatedAt

# Verify forecaster is running
kubectl get pods -l app=kedastral-forecaster-api -n multi-app-redis
```

### Pods not scaling
- Check KEDA logs: `kubectl logs -n keda-system -l app=keda-operator`
- Check ScaledObjects: `kubectl describe scaledobject -n multi-app-redis`
- Verify scaler is reachable: `kubectl get svc -n multi-app-redis`

## Cleanup

```bash
./cleanup.sh
```

This will:
1. Delete multi-app-redis namespace (removes all resources)
2. Optionally delete KEDA
3. Optionally delete Prometheus
4. Optionally stop minikube

## Next Steps

Once you understand the multi-app Redis pattern:

1. **Add more workloads**: Create additional forecaster + ScaledObject pairs
2. **Tune independently**: Each workload can have different models, policies, lead times
3. **Test HA**: Scale forecasters to multiple replicas
4. **Add persistence**: Configure Redis with PersistentVolume
5. **Monitor at scale**: Track forecast accuracy across all workloads
6. **Implement ARIMA**: Try different models for different workload types

## Resources

- [Kedastral Documentation](../../README.md)
- [Redis Documentation](https://redis.io/docs/)
- [KEDA External Scaler](https://keda.sh/docs/scalers/external/)
- [Prometheus Query Basics](https://prometheus.io/docs/prometheus/latest/querying/basics/)
