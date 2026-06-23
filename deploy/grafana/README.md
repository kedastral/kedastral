# Grafana Dashboard

`kedastral-dashboard.json` visualizes Kedastral's predictive autoscaling using the
`kedastral_*` Prometheus metrics exported by the forecaster and scaler.

It is workload-agnostic and driven by two template variables:

- **Data source** — the Prometheus data source to query.
- **Workload** — populated from `label_values(kedastral_desired_replicas, workload)`.

## Panels

- **Forecast** — predicted metric value; planned replicas (forecaster) vs returned
  (scaler) vs HPA desired; forecast age (staleness) for both forecaster and scaler.
- **Forecaster pipeline** — average collect/predict/capacity stage durations and error rate.
- **Scaler** — gRPC request rate, gRPC p99 latency, and forecast-fetch latency/errors.

The "HPA desired" series uses `kube_horizontalpodautoscaler_status_desired_replicas`
from kube-state-metrics; it is optional and simply stays empty if that metric is absent.

## Import

In Grafana: **Dashboards → New → Import**, upload `kedastral-dashboard.json`, then pick
your Prometheus data source. Or provision it via a dashboards ConfigMap / the Grafana
provisioning directory.
