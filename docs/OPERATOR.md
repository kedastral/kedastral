# Kedastral Operator

The operator lets you configure Kedastral declaratively with Kubernetes custom
resources instead of flags or a static config file. It is **embedded in the
forecaster**: when operator mode is enabled, the forecaster runs a controller that
watches `ForecastPolicy` and `DataSource` resources and:

1. Starts, updates, or stops an in-process forecast loop for each `ForecastPolicy`.
2. Generates and owns a KEDA `ScaledObject` per policy, wiring the external scaler
   to the policy's `scaleTargetRef`.
3. Reports forecast state back on the policy's `.status`.

No separate operator deployment is needed, and no per-workload pods are created — all
workloads run as goroutines inside the forecaster, exactly like multi-workload mode.

## Custom resources

### DataSource

Describes a metrics backend. Multiple policies in the same namespace can share one.

```yaml
apiVersion: kedastral.io/v1alpha1
kind: DataSource
metadata:
  name: prometheus
spec:
  type: prometheus        # prometheus | victoriametrics | http
  config:                 # adapter-specific, passed straight to the adapter factory
    url: http://prometheus.monitoring:9090
    query: sum(rate(http_requests_total{app="web-api"}[1m]))
```

### ForecastPolicy

Describes a workload to forecast and scale. See
[`deploy/examples/forecastpolicy.yaml`](../deploy/examples/forecastpolicy.yaml) for a
full example including the ARIMA model. The spec maps directly onto the forecaster's
workload configuration and capacity planner; `model.type` selects `baseline`, `arima`,
`sarima`, or `byom`.

The controller derives the forecast workload key as `<namespace>-<name>`, which is
also placed in the generated ScaledObject trigger's `workload` metadata so the scaler
queries the matching snapshot.

## Enabling operator mode

Operator mode requires:

- The CRDs installed (shipped in the Helm chart under `crds/`, applied automatically
  on `helm install`).
- [KEDA](https://keda.sh) installed in the cluster (the controller creates
  `keda.sh/v1alpha1 ScaledObject` resources).
- RBAC for the forecaster ServiceAccount (created by the chart when operator mode is on).

```bash
helm install kedastral deploy/helm/kedastral \
  --set forecaster.operator.enabled=true \
  --set forecaster.operator.scalerAddress=kedastral-scaler:50051
```

Then apply your resources:

```bash
kubectl apply -f deploy/examples/datasource.yaml
kubectl apply -f deploy/examples/forecastpolicy.yaml
kubectl get forecastpolicies
```

## How it reconciles

- Creating/updating a `ForecastPolicy` resolves its `DataSourceRef`, translates the
  spec into a workload configuration, (re)starts the forecast loop, and reconciles the
  ScaledObject.
- Editing a `DataSource` re-triggers every policy in its namespace that references it.
- Deleting a `ForecastPolicy` stops its forecast loop and removes its snapshot; the
  ScaledObject is garbage-collected via its owner reference.
- If the referenced `DataSource` is missing or the spec is invalid, the policy's
  `Ready` condition is set to `False` with the reason, and reconciliation is retried.

## Status

`kubectl get forecastpolicy web-api -o yaml` reports `currentReplicas`,
`desiredReplicas` (peak over the horizon), `lastForecastTime`, the generated
`scaledObjectName`, and a `Ready` condition.

## Regenerating CRDs

CRD manifests and deepcopy code are generated from the Go types in
`pkg/api/v1alpha1`:

```bash
make generate    # deepcopy methods
make manifests   # CRD YAML into deploy/helm/kedastral/crds/
```
