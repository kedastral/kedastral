# Kedastral

*Predict tomorrow's load, scale today.*

[![Release](https://img.shields.io/github/v/release/kedastral/kedastral?color=blue)](https://github.com/kedastral/kedastral/releases/latest)
[![Go Reference](https://pkg.go.dev/badge/github.com/kedastral/kedastral.svg)](https://pkg.go.dev/github.com/kedastral/kedastral)
[![Go Report Card](https://goreportcard.com/badge/github.com/kedastral/kedastral)](https://goreportcard.com/report/github.com/kedastral/kedastral)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

---

## Overview

**Kedastral** is an open-source **predictive autoscaling companion for [KEDA](https://keda.sh/)** that enables Kubernetes workloads to scale proactively based on forecasted demand.

Where **KEDA** reacts to what *has already happened*, **Kedastral** predicts *what will happen next* — keeping applications responsive and cost-efficient during traffic surges.

**Key Features:**
- 🔮 **Predictive scaling** — Forecast demand and scale before spikes arrive
- ⚙️ **KEDA-native** — Implements KEDA External Scaler gRPC protocol
- 📈 **Prometheus integration** — Pull metrics from Prometheus for forecasting
- 🧠 **Multiple models** — Statistical baseline + ARIMA time-series forecasting
- 💾 **HA-ready** — In-memory or Redis storage for high availability
- 🚀 **Fast & efficient** — Built in Go, minimal footprint
- 🔐 **Data stays local** — All forecasting happens inside your cluster

---

## How It Works

```mermaid
flowchart LR
    A[Prometheus] --> B[Forecaster]
    B --> C[Scaler]
    C --> D[KEDA]
    D --> E[HPA]
    E --> F[Your Workload]
```

1. **Forecaster** collects metrics from Prometheus and generates predictions
2. **Scaler** fetches forecasts and implements KEDA External Scaler protocol
3. **KEDA** receives desired replicas and updates the HPA
4. **Workload** scales proactively before demand arrives

---

## Quick Start

```bash
# Build
git clone https://github.com/kedastral/kedastral.git
cd kedastral
make build

# Deploy to Kubernetes
kubectl apply -f examples/deployment.yaml
kubectl apply -f examples/scaled-object.yaml

# Monitor
kubectl logs -l component=forecaster -f
```

**See the [Quick Start Guide](docs/QUICKSTART.md) for detailed instructions.**

---

## Use Cases

Kedastral is **domain-agnostic** and works for any workload with predictable patterns:

| Domain | Example Signals | Benefit |
|--------|----------------|---------|
| E-commerce | Request rate, time of day | Pre-scale before sales campaigns |
| Video streaming | Viewer counts, release schedules | Pre-scale for new show launches |
| Banking & fintech | Batch job schedules, queue lag | Handle end-of-month processing |
| IoT ingestion | Device counts, telemetry spikes | Absorb sensor data bursts |
| SaaS APIs | RPS, active sessions | Prevent latency from scaling lag |

---

## Documentation

### Getting Started
- **[Quick Start Guide](docs/QUICKSTART.md)** - Get running in 10 minutes
- **[Architecture Overview](docs/ARCHITECTURE.md)** - System design and components
- **[Configuration Reference](docs/CONFIGURATION.md)** - All flags and environment variables
- **[Deployment Guide](docs/DEPLOYMENT.md)** - Production deployment patterns

### Components
- **[Forecaster](cmd/forecaster/)** - Prediction engine and capacity planner
- **[Scaler](cmd/scaler/)** - KEDA External Scaler implementation

### Deep Dives
- **[Forecasting Models](docs/models/)** - Baseline and ARIMA models
- **[Capacity Planning](docs/planner/)** - Replica calculation and tuning
- **[Observability](docs/OBSERVABILITY.md)** - Metrics and monitoring
- **[Security Audit](docs/SECURITY_AUDIT.md)** - Security review results

### Examples
- **[Deployment Examples](examples/)** - Kubernetes manifests and usage guide

---

## Current Status (v0.1)

**Production-ready core components:**

| Component | Status |
|-----------|--------|
| Forecaster (HTTP API + Prometheus) | ✅ |
| Scaler (KEDA External Scaler) | ✅ |
| Baseline forecasting model | ✅ |
| ARIMA forecasting model | ✅ |
| In-memory storage | ✅ |
| Redis storage (HA) | ✅ |
| Quantile forecasting | ✅ |
| Multi-workload support | ✅ |
| TLS support | ✅ |
| Prometheus metrics | ✅ |
| Comprehensive tests (81 tests) | ✅ |
| Docker support | ✅ |
| Kubernetes examples | ✅ |

**Planned for v0.2+:**
- Helm charts
- Additional adapters (Kafka, HTTP)
- Advanced ML models (Prophet, SARIMA)
- Grafana dashboards
- CRDs and Operator

---

## Tech Stack

- **Language:** Go 1.25+
- **Forecaster API:** HTTP/REST
- **Scaler API:** gRPC (KEDA External Scaler protocol)
- **Storage:** In-memory / Redis
- **Metrics:** Prometheus
- **Models:** Baseline (statistical), ARIMA (time-series)

---

## Installation

### Prerequisites

- Kubernetes cluster (v1.20+)
- KEDA installed ([installation guide](https://keda.sh/docs/latest/deploy/))
- Prometheus running in the cluster

### From Source

```bash
git clone https://github.com/kedastral/kedastral.git
cd kedastral
make build
kubectl apply -f examples/deployment.yaml
kubectl apply -f examples/scaled-object.yaml
```

### Docker

```bash
docker pull kedastral/forecaster:latest
docker pull kedastral/scaler:latest
```

See the [Deployment Guide](docs/DEPLOYMENT.md) for production setup.

---

## Example Configuration

**Forecaster:**
```bash
./bin/forecaster \
  --workload=my-api \
  --metric=http_rps \
  --prom-url=http://prometheus:9090 \
  --prom-query='sum(rate(http_requests_total{service="my-api"}[1m]))' \
  --target-per-pod=100 \
  --headroom=1.2 \
  --min=2 \
  --max=50 \
  --horizon=30m \
  --model=baseline
```

**Scaler:**
```bash
./bin/scaler \
  --forecaster-url=http://kedastral-forecaster:8081 \
  --lead-time=10m
```

**KEDA ScaledObject:**
```yaml
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: my-api-scaledobject
spec:
  scaleTargetRef:
    name: my-api
  triggers:
    - type: external
      metadata:
        scalerAddress: kedastral-scaler:50051
        workload: my-api
```

See the [Configuration Reference](docs/CONFIGURATION.md) for all options.

---

## Observability

Both components expose Prometheus metrics:

**Forecaster Metrics:**
- `kedastral_predicted_value` - Current predicted metric value
- `kedastral_desired_replicas` - Desired replica count
- `kedastral_forecast_age_seconds` - Forecast staleness

**Scaler Metrics:**
- `kedastral_scaler_desired_replicas_returned` - Replicas returned to KEDA
- `kedastral_scaler_forecast_fetch_duration_seconds` - Fetch latency

See the [Observability Guide](docs/OBSERVABILITY.md) for complete metrics reference.

---

## Roadmap

| Version | Key Features | Status |
|---------|--------------|--------|
| **v0.1** | Forecaster + Scaler + Baseline/ARIMA + Redis | ✅ Complete |
| **v0.2** | Helm charts + Grafana dashboards | 🔄 Planned |
| **v0.3** | CRDs + Operator + Additional adapters | 🔄 Planned |
| **v1.0** | Production hardening + Model registry | 🔄 Planned |

---

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

**Ways to contribute:**
- Report bugs and request features via [GitHub Issues](https://github.com/kedastral/kedastral/issues)
- Submit pull requests for bug fixes or new features
- Add new adapters (Kafka, HTTP, custom data sources)
- Add new forecasting models
- Improve documentation

---

## License & Governance

- **License:** Apache-2.0
- **Repository:** [github.com/kedastral/kedastral](https://github.com/kedastral/kedastral)
- **Maintainers:** Community-governed, CNCF-style steering model

---

## Support

- **Documentation:** [docs/](docs/)
- **Examples:** [examples/](examples/)
- **Issues:** [GitHub Issues](https://github.com/kedastral/kedastral/issues)
- **Discussions:** [GitHub Discussions](https://github.com/kedastral/kedastral/discussions)

---

## Acknowledgments

Built with:
- [KEDA](https://keda.sh/) - Kubernetes Event-driven Autoscaling
- [Prometheus](https://prometheus.io/) - Monitoring and alerting
- [Go](https://go.dev/) - Programming language

---

**Made with predictive scaling in mind.**
