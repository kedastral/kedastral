# Deployment Guide

This guide covers production deployment patterns for Kedastral on Kubernetes.

## Prerequisites

Before deploying Kedastral, ensure you have:

- **Kubernetes cluster** (v1.20+)
- **KEDA** installed ([installation guide](https://keda.sh/docs/latest/deploy/))
- **Prometheus** running in the cluster
- **kubectl** configured to access your cluster

## Deployment Patterns

### Pattern 1: Single-Instance (Development)

**Architecture:**
```
Forecaster (memory storage) ←→ Scaler ←→ KEDA
```

**Best for:**
- Development and testing
- Single workload
- Non-critical workloads

**Pros:**
- Simple setup, no dependencies
- Fast startup
- Easy debugging

**Cons:**
- No high availability
- No persistence across restarts
- Single point of failure

**Deploy:**
```bash
kubectl apply -f examples/deployment.yaml
kubectl apply -f examples/scaled-object.yaml
```

### Pattern 2: High-Availability (Production)

**Architecture:**
```
Redis ←→ Forecaster (2+ replicas) ←→ Scaler (2+ replicas) ←→ KEDA
```

**Best for:**
- Production workloads
- Multiple workloads
- Mission-critical applications

**Pros:**
- High availability
- Persistent forecasts
- Horizontal scaling
- Shared state

**Cons:**
- Redis dependency
- Slightly more complex

**Deploy:**
```bash
# Deploy Redis first
kubectl apply -f examples/redis.yaml

# Deploy Kedastral with Redis
kubectl apply -f examples/deployment-redis.yaml
kubectl apply -f examples/scaled-object.yaml
```

## Storage Backends

### In-Memory Storage (Default)

**Configuration:**
```yaml
env:
  - name: STORAGE
    value: "memory"
```

**When to use:**
- Development
- Testing
- Single forecaster instance
- Short-lived forecasts

**Limitations:**
- No persistence across pod restarts
- Cannot share state between replicas
- No HA support

### Redis Storage

**Configuration:**
```yaml
env:
  - name: STORAGE
    value: "redis"
  - name: REDIS_ADDR
    value: "redis:6379"
  - name: REDIS_PASSWORD
    valueFrom:
      secretKeyRef:
        name: redis-secret
        key: password
  - name: REDIS_DB
    value: "0"
  - name: REDIS_TTL
    value: "1h"
```

**When to use:**
- Production deployments
- Multiple forecaster replicas
- Persistent forecasts
- HA requirements

**Benefits:**
- Shared state across forecaster replicas
- Persistence across restarts
- TTL-based expiration
- Horizontal scaling

**Redis Setup:**

1. **Deploy Redis:**
   ```bash
   # Using Helm
   helm repo add bitnami https://charts.bitnami.com/bitnami
   helm install redis bitnami/redis --set auth.password=yourpassword

   # Or use manifest
   kubectl apply -f examples/redis.yaml
   ```

2. **Create Secret:**
   ```yaml
   apiVersion: v1
   kind: Secret
   metadata:
     name: redis-secret
   stringData:
     password: "yourpassword"
   ```

3. **Configure Forecaster:**
   ```yaml
   env:
     - name: STORAGE
       value: "redis"
     - name: REDIS_ADDR
       value: "redis-master:6379"
     - name: REDIS_PASSWORD
       valueFrom:
         secretKeyRef:
           name: redis-secret
           key: password
   ```

## Multi-Workload Deployment

**Security Note:** Kedastral uses a one-workload-per-forecaster architecture for security and isolation. Each forecaster instance manages exactly one workload.

For multiple workloads, deploy multiple forecaster instances. The recommended approach is using Helm to template deployments.

### Option 1: Manual Multiple Deployments

Deploy separate forecaster instances for each workload:

```yaml
---
# Forecaster for frontend
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kedastral-forecaster-frontend
spec:
  replicas: 1
  selector:
    matchLabels:
      app: kedastral
      workload: frontend
  template:
    metadata:
      labels:
        app: kedastral
        workload: frontend
    spec:
      containers:
      - name: forecaster
        image: kedastral/forecaster:latest
        env:
        - name: WORKLOAD
          value: "api-frontend"
        - name: METRIC
          value: "http_rps"
        - name: PROM_QUERY
          value: 'sum(rate(http_requests_total{service="frontend"}[1m]))'
        - name: MODEL
          value: "baseline"
        - name: TARGET_PER_POD
          value: "100"
        - name: MIN_REPLICAS
          value: "2"
        - name: MAX_REPLICAS
          value: "50"
        ports:
        - containerPort: 8081
---
apiVersion: v1
kind: Service
metadata:
  name: kedastral-forecaster-frontend
spec:
  ports:
  - port: 8081
  selector:
    app: kedastral
    workload: frontend
---
# Forecaster for backend
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kedastral-forecaster-backend
spec:
  replicas: 1
  selector:
    matchLabels:
      app: kedastral
      workload: backend
  template:
    metadata:
      labels:
        app: kedastral
        workload: backend
    spec:
      containers:
      - name: forecaster
        image: kedastral/forecaster:latest
        env:
        - name: WORKLOAD
          value: "api-backend"
        - name: METRIC
          value: "http_rps"
        - name: PROM_QUERY
          value: 'sum(rate(http_requests_total{service="backend"}[1m]))'
        - name: MODEL
          value: "sarima"
        - name: SARIMA_S
          value: "24"
        - name: TARGET_PER_POD
          value: "200"
        - name: MIN_REPLICAS
          value: "3"
        - name: MAX_REPLICAS
          value: "100"
        ports:
        - containerPort: 8081
---
apiVersion: v1
kind: Service
metadata:
  name: kedastral-forecaster-backend
spec:
  ports:
  - port: 8081
  selector:
    app: kedastral
    workload: backend
```

### Option 2: Helm Template (Recommended)

Create a Helm chart that templates multiple forecasters:

**values.yaml:**
```yaml
workloads:
  - name: frontend
    metric: http_rps
    promQuery: 'sum(rate(http_requests_total{service="frontend"}[1m]))'
    model: baseline
    targetPerPod: 100
    minReplicas: 2
    maxReplicas: 50

  - name: backend
    metric: http_rps
    promQuery: 'sum(rate(http_requests_total{service="backend"}[1m]))'
    model: sarima
    sarimaS: 24
    targetPerPod: 200
    minReplicas: 3
    maxReplicas: 100
```

**templates/forecaster-deployment.yaml:**
```yaml
{{- range .Values.workloads }}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kedastral-forecaster-{{ .name }}
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
        image: {{ $.Values.image.repository }}:{{ $.Values.image.tag }}
        env:
        - name: WORKLOAD
          value: {{ .name | quote }}
        - name: METRIC
          value: {{ .metric | quote }}
        {{- if .promQuery }}
        - name: PROM_QUERY
          value: {{ .promQuery | quote }}
        {{- end }}
        - name: MODEL
          value: {{ .model | default "baseline" | quote }}
        {{- if eq .model "sarima" }}
        - name: SARIMA_S
          value: {{ .sarimaS | default 24 | quote }}
        {{- end }}
        - name: TARGET_PER_POD
          value: {{ .targetPerPod | quote }}
        - name: MIN_REPLICAS
          value: {{ .minReplicas | quote }}
        - name: MAX_REPLICAS
          value: {{ .maxReplicas | quote }}
        ports:
        - containerPort: 8081
---
apiVersion: v1
kind: Service
metadata:
  name: kedastral-forecaster-{{ .name }}
spec:
  ports:
  - port: 8081
  selector:
    app: kedastral
    component: forecaster
    workload: {{ .name }}
{{- end }}
```

**Deploy:**
```bash
helm install kedastral ./charts/kedastral -f values.yaml
```

### Create ScaledObjects for Each Workload

```yaml
---
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: frontend-scaledobject
spec:
  scaleTargetRef:
    name: api-frontend
  triggers:
    - type: external
      metadata:
        scalerAddress: kedastral-scaler:50051
        workload: api-frontend
---
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: backend-scaledobject
spec:
  scaleTargetRef:
    name: api-backend
  triggers:
    - type: external
      metadata:
        scalerAddress: kedastral-scaler:50051
        workload: api-backend
```

**Benefits of this architecture:**
- **Security:** No file system access, no config file vulnerabilities
- **Isolation:** Workload failures don't affect other workloads
- **Scalability:** Scale forecasters independently per workload
- **Kubernetes-native:** Standard ConfigMap → env var pattern

## Resource Requirements

### Forecaster

**Recommended:**
```yaml
resources:
  requests:
    memory: "256Mi"
    cpu: "100m"
  limits:
    memory: "512Mi"
    cpu: "500m"
```

**Factors:**
- Memory scales with window size and number of workloads
- ARIMA model requires ~5MB per workload
- Baseline model requires ~1MB per workload

### Scaler

**Recommended:**
```yaml
resources:
  requests:
    memory: "64Mi"
    cpu: "50m"
  limits:
    memory: "128Mi"
    cpu: "200m"
```

**Factors:**
- Lightweight gRPC server
- Memory overhead minimal
- CPU scales with KEDA polling frequency

## High Availability Configuration

### Forecaster HA

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kedastral-forecaster
spec:
  replicas: 2  # or more
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 1
      maxSurge: 1
  template:
    spec:
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
            - weight: 100
              podAffinityTerm:
                labelSelector:
                  matchLabels:
                    component: forecaster
                topologyKey: kubernetes.io/hostname
      containers:
      - name: forecaster
        env:
          - name: STORAGE
            value: "redis"  # Required for HA
          - name: REDIS_ADDR
            value: "redis-master:6379"
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
```

### Scaler HA

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kedastral-scaler
spec:
  replicas: 2  # or more
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 1
      maxSurge: 1
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
```

## Security

### TLS Configuration

Enable TLS for forecaster HTTP API:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kedastral-forecaster
spec:
  template:
    spec:
      containers:
      - name: forecaster
        env:
          - name: TLS_ENABLED
            value: "true"
          - name: TLS_CERT_FILE
            value: "/etc/certs/tls.crt"
          - name: TLS_KEY_FILE
            value: "/etc/certs/tls.key"
          - name: TLS_CA_FILE
            value: "/etc/certs/ca.crt"
        volumeMounts:
          - name: certs
            mountPath: /etc/certs
            readOnly: true
      volumes:
        - name: certs
          secret:
            secretName: kedastral-tls
```

Enable TLS for scaler → forecaster connection:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kedastral-scaler
spec:
  template:
    spec:
      containers:
      - name: scaler
        env:
          - name: FORECASTER_URL
            value: "https://kedastral-forecaster:8081"
          - name: TLS_ENABLED
            value: "true"
          - name: TLS_CERT_FILE
            value: "/etc/certs/tls.crt"
          - name: TLS_KEY_FILE
            value: "/etc/certs/tls.key"
          - name: TLS_CA_FILE
            value: "/etc/certs/ca.crt"
        volumeMounts:
          - name: certs
            mountPath: /etc/certs
            readOnly: true
      volumes:
        - name: certs
          secret:
            secretName: kedastral-tls
```

### RBAC

Kedastral requires minimal RBAC permissions:

```yaml
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kedastral
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kedastral-role
rules:
  # Forecaster needs no special permissions (only talks to Prometheus)
  # Scaler needs no special permissions (KEDA talks to it, not vice versa)
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kedastral-binding
subjects:
  - kind: ServiceAccount
    name: kedastral
    namespace: default
roleRef:
  kind: ClusterRole
  name: kedastral-role
  apiGroup: rbac.authorization.k8s.io
```

**Note:** Kedastral requires minimal permissions. The forecaster only needs network access to Prometheus, and the scaler only exposes a gRPC API for KEDA to consume.

### Network Policies

Restrict network access:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: kedastral-forecaster
spec:
  podSelector:
    matchLabels:
      component: forecaster
  policyTypes:
    - Ingress
    - Egress
  ingress:
    # Allow scaler to fetch forecasts
    - from:
      - podSelector:
          matchLabels:
            component: scaler
      ports:
        - protocol: TCP
          port: 8081
  egress:
    # Allow Prometheus queries
    - to:
      - podSelector:
          matchLabels:
            app: prometheus
      ports:
        - protocol: TCP
          port: 9090
    # Allow Redis (if used)
    - to:
      - podSelector:
          matchLabels:
            app: redis
      ports:
        - protocol: TCP
          port: 6379
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: kedastral-scaler
spec:
  podSelector:
    matchLabels:
      component: scaler
  policyTypes:
    - Ingress
    - Egress
  ingress:
    # Allow KEDA to call gRPC
    - from:
      - namespaceSelector:
          matchLabels:
            name: keda
      ports:
        - protocol: TCP
          port: 50051
  egress:
    # Allow forecaster HTTP calls
    - to:
      - podSelector:
          matchLabels:
            component: forecaster
      ports:
        - protocol: TCP
          port: 8081
```

## Monitoring

### Prometheus ServiceMonitor

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: kedastral
spec:
  selector:
    matchLabels:
      app: kedastral
  endpoints:
    - port: http
      path: /metrics
      interval: 30s
```

### Health Checks

Both components expose `/healthz` endpoints:

```bash
# Check forecaster health
curl http://kedastral-forecaster:8081/healthz

# Check scaler health
curl http://kedastral-scaler:8082/healthz
```

## Troubleshooting

### Forecaster Issues

**Problem:** Forecaster not collecting metrics

```bash
# Check logs
kubectl logs -l component=forecaster

# Test Prometheus connectivity
kubectl exec -it deployment/kedastral-forecaster -- wget -O- http://prometheus:9090/api/v1/query?query=up

# Check configuration
kubectl get configmap kedastral-config -o yaml
```

**Problem:** Forecasts not being stored

```bash
# Check Redis connectivity (if using Redis)
kubectl exec -it deployment/kedastral-forecaster -- redis-cli -h redis PING

# Verify snapshot API
curl "http://kedastral-forecaster:8081/forecast/current?workload=my-api"
```

### Scaler Issues

**Problem:** KEDA not receiving metrics

```bash
# Check scaler logs
kubectl logs -l component=scaler

# Test gRPC endpoint
kubectl port-forward svc/kedastral-scaler 50051:50051
grpcurl -plaintext localhost:50051 externalscaler.ExternalScaler/GetMetrics

# Check ScaledObject status
kubectl describe scaledobject my-api-scaledobject
```

**Problem:** Stale forecasts

```bash
# Check forecast age
curl "http://kedastral-forecaster:8081/forecast/current?workload=my-api" | jq '.generatedAt'

# Check scaler logs for fetch errors
kubectl logs -l component=scaler | grep "fetch error"
```

## Upgrade Strategy

### Rolling Update (Zero Downtime)

1. **Update forecaster:**
   ```bash
   kubectl set image deployment/kedastral-forecaster forecaster=kedastral/forecaster:v0.2.0
   kubectl rollout status deployment/kedastral-forecaster
   ```

2. **Update scaler:**
   ```bash
   kubectl set image deployment/kedastral-scaler scaler=kedastral/scaler:v0.2.0
   kubectl rollout status deployment/kedastral-scaler
   ```

### Canary Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kedastral-forecaster-canary
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: forecaster
        image: kedastral/forecaster:v0.2.0-rc
```

Test canary, then promote:
```bash
kubectl scale deployment/kedastral-forecaster-canary --replicas=0
kubectl set image deployment/kedastral-forecaster forecaster=kedastral/forecaster:v0.2.0
```

## Next Steps

- **Configuration**: See [CONFIGURATION.md](CONFIGURATION.md) for all options
- **Observability**: Set up monitoring with [OBSERVABILITY.md](OBSERVABILITY.md)
- **Tuning**: Optimize capacity planning in [planner/tuning.md](planner/tuning.md)
- **Examples**: See [examples/README.md](../examples/README.md) for complete manifests
