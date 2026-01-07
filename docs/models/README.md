# Kedastral Forecasting Models

This directory contains documentation for Kedastral's forecasting models. Each model uses different algorithms and is suited for different workload patterns.

## Available Models

### 🎯 [Baseline Model](./baseline.md) — **Recommended for Most Users**

Fast, zero-configuration forecasting combining trend detection, momentum, and seasonality learning.

**Best for:**
- Hourly or daily traffic patterns
- Recurring spikes (every 15min, 30min, hourly)
- Workloads with 3-24 hours of historical data
- Quick setup without tuning

**Quick start:**
```bash
MODEL=baseline
WINDOW=3h
HORIZON=30m
```

[→ Full Baseline Documentation](./baseline.md)

---

### 📊 [ARIMA Model](./arima.md) — **Advanced Statistical Forecasting**

AutoRegressive Integrated Moving Average model for complex patterns and long-term trends.

**Best for:**
- Weekly patterns (weekday vs weekend)
- Monthly cycles (billing spikes, payroll)
- Multi-day autocorrelation
- Workloads with 1-7 days of historical data

**Quick start:**
```bash
MODEL=arima
ARIMA_P=7          # Weekly lookback
ARIMA_D=1          # Remove trend
ARIMA_Q=1          # Error correction
WINDOW=7d
HORIZON=24h
```

[→ Full ARIMA Documentation](./arima.md)

---

### 🌊 [SARIMA Model](./sarima.md) — **Seasonal ARIMA for Repeating Patterns**

Seasonal AutoRegressive Integrated Moving Average model for workloads with strong recurring patterns.

**Best for:**
- Hourly data with daily patterns (rush hours, business hours)
- Daily data with weekly patterns (weekday vs weekend)
- Strong repeating seasonal cycles
- Workloads with both trend AND seasonality
- At least 2 full seasonal periods of data

**Quick start:**
```bash
MODEL=sarima
SARIMA_P=1 SARIMA_D=1 SARIMA_Q=1    # Non-seasonal
SARIMA_SP=1 SARIMA_SD=1 SARIMA_SQ=1 # Seasonal
SARIMA_S=24                          # Daily pattern (hourly data)
WINDOW=168h
HORIZON=30m
```

[→ Full SARIMA Documentation](./sarima.md)

---

## Model Comparison

| Feature | Baseline | ARIMA | SARIMA |
|---------|----------|-------|--------|
| **Setup Complexity** | ✅ Zero config | ⚙️ Medium (p,d,q) | 🔧 High (p,d,q,P,D,Q,s) |
| **Pattern Detection** | Intra-day | Multi-day trends | Seasonal + trends |
| **Training Speed** | ⚡ ~10ms | 🔄 ~100ms | 🐌 ~500ms |
| **Prediction Speed** | ⚡ ~10ms | 🔄 ~100ms | 🔄 ~100ms |
| **Min Training Data** | 3 hours | 1-7 days | 2 seasonal periods (2*s) |
| **Optimal Data Window** | 3-24 hours | 1-7 days | 1-2 weeks |
| **Memory Usage** | Low (~10MB) | Medium (~30MB) | High (~100MB) |
| **Accuracy** | Good | Higher | Highest (seasonal) |
| **Seasonality Handling** | Basic (hour-of-day) | ❌ | ✅ Statistical |
| **Best Use Case** | Daily cycles | Weekly trends | Daily/weekly patterns |

## Choosing a Model

### Start with Baseline if:

- ✅ You're new to Kedastral
- ✅ Your workload has predictable hourly/daily patterns
- ✅ You want zero-configuration setup
- ✅ You have < 24 hours of historical data
- ✅ Prediction latency matters

**Example scenarios:**
- API with lunch-hour traffic spikes
- Batch jobs running every 30 minutes
- Microservice with business-hours peak (9am-5pm)

### Switch to ARIMA if:

- 📈 Baseline predictions aren't accurate enough
- 📅 Your pattern spans multiple days (weekday vs weekend)
- 🔬 You need statistical rigor and parameter control
- 💾 You have 1+ weeks of training data
- ⏱️ You can tolerate ~200ms prediction latency

**Example scenarios:**
- B2B SaaS with weekend downtime
- Payment processor with month-end spikes
- Gaming platform with weekly maintenance windows

### Use SARIMA if:

- 🌊 Your workload has **strong repeating patterns** (daily, weekly)
- 📊 Both trend AND seasonality present
- 🔬 You need the highest accuracy for seasonal forecasting
- 💾 You have 2+ weeks of training data (2+ seasonal periods)
- 🎯 Clear seasonal period (hourly→daily, daily→weekly)
- 🧮 Willing to tune 7 parameters (p,d,q,P,D,Q,s)

**Example scenarios:**
- Web API with consistent daily rush hours (9am, 12pm, 5pm)
- E-commerce with weekday business hours and weekend lulls
- Batch processing with daily/weekly scheduled jobs
- Gaming servers with evening/weekend player spikes
- APIs with international timezone patterns

### Decision Tree

```
Does your workload have patterns?
│
├─ NO → Use Baseline (it's fast and adaptive)
│
└─ YES → Is the pattern seasonal (repeating)?
    │
    ├─ NO (just trending) → Use ARIMA
    │
    └─ YES (repeating) → Do you have clear seasonal period?
        │
        ├─ NO → Use Baseline (built-in hour-of-day)
        │
        └─ YES (e.g., s=24, s=168) → Use SARIMA
```

## Configuration Reference

### Shared Parameters

These apply to all models:

| Variable | Flag | Default | Description |
|----------|------|---------|-------------|
| `MODEL` | `--model` | `baseline` | Model type: `baseline`, `arima`, or `sarima` |
| `METRIC` | `--metric` | *required* | Metric name to forecast |
| `STEP` | `--step` | `1m` | Time between predictions |
| `HORIZON` | `--horizon` | `30m` | How far ahead to predict |
| `WINDOW` | `--window` | `30m` | Historical data for training |
| `INTERVAL` | `--interval` | `30s` | How often to run forecast loop |

### ARIMA-Specific Parameters

| Variable | Flag | Default | Description |
|----------|------|---------|-------------|
| `ARIMA_P` | `--arima-p` | `0` (auto→1) | AutoRegressive order |
| `ARIMA_D` | `--arima-d` | `0` (auto→1) | Differencing order |
| `ARIMA_Q` | `--arima-q` | `0` (auto→1) | Moving Average order |

### SARIMA-Specific Parameters

| Variable | Flag | Default | Description |
|----------|------|---------|-------------|
| `SARIMA_P` | `--sarima-p` | `0` (auto→1) | Non-seasonal AR order |
| `SARIMA_D` | `--sarima-d` | `0` (auto→1) | Non-seasonal differencing |
| `SARIMA_Q` | `--sarima-q` | `0` (auto→1) | Non-seasonal MA order |
| `SARIMA_SP` | `--sarima-sp` | `1` | Seasonal AR order (P) |
| `SARIMA_SD` | `--sarima-sd` | `1` | Seasonal differencing (D) |
| `SARIMA_SQ` | `--sarima-sq` | `1` | Seasonal MA order (Q) |
| `SARIMA_S` | `--sarima-s` | `24` | Seasonal period (e.g., 24, 168) |

## Quick Start Examples

### Example 1: Baseline for Hourly Spikes

```yaml
# deploy/examples/baseline-hourly-spikes.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kedastral-forecaster-config
data:
  MODEL: baseline
  METRIC: http_requests_per_second
  WINDOW: 6h
  STEP: 1m
  HORIZON: 30m
  INTERVAL: 30s
```

### Example 2: ARIMA for Weekly Pattern

```yaml
# deploy/examples/arima-weekly-pattern.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kedastral-forecaster-config
data:
  MODEL: arima
  METRIC: queue_depth
  ARIMA_P: 7
  ARIMA_D: 1
  ARIMA_Q: 1
  WINDOW: 14d
  STEP: 1h
  HORIZON: 24h
  INTERVAL: 5m
```

### Example 3: SARIMA for Daily Seasonal Pattern

```yaml
# deploy/examples/sarima-daily-pattern.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kedastral-forecaster-config
data:
  MODEL: sarima
  METRIC: http_requests_per_second
  SARIMA_P: 1
  SARIMA_D: 1
  SARIMA_Q: 1
  SARIMA_SP: 1
  SARIMA_SD: 1
  SARIMA_SQ: 1
  SARIMA_S: 24      # Hourly data, daily pattern
  WINDOW: 168h      # 1 week
  STEP: 1m
  HORIZON: 30m
  INTERVAL: 5m
```

## Monitoring Model Performance

### Key Metrics to Watch

```promql
# Prediction latency (Histogram)
kedastral_model_predict_seconds

# Training/prediction errors (Counter)
kedastral_errors_total{component="model"}

# Forecast age (Gauge - should be < interval)
kedastral_forecast_age_seconds

# Desired replicas (Gauge - should vary predictively)
kedastral_desired_replicas
```

### Grafana Dashboard

See [deploy/grafana/kedastral-dashboard.json](../../deploy/grafana/kedastral-dashboard.json) for:
- Model prediction latency over time
- Forecast vs actual comparison
- Training success rate
- Replica count prediction accuracy

## Debugging Tips

### Problem: Flat Predictions (Not Varying)

**Baseline:**
1. Check if training data has clear patterns
2. Increase `WINDOW` to capture more pattern occurrences
3. Verify Prometheus query returns varying data
4. Check logs for "model training skipped" (means no patterns learned)

**ARIMA:**
1. Verify sufficient data points (min: max(p+d, q+d, 10))
2. Try increasing `WINDOW`
3. Check for "numerical instability" errors
4. Reduce `p` or `q` if data is sparse

### Problem: Predictions Lag Behind Reality

**Both models:**
1. Decrease `WINDOW` for faster trend adaptation
2. Increase `LEAD_TIME` in capacity planning policy
3. Decrease `INTERVAL` to forecast more frequently
4. For ARIMA: reduce `p` (less historical dependency)

### Problem: Training Errors

**Check logs:**
```bash
kubectl logs deployment/kedastral-forecaster | grep -i error
```

**Common errors:**
- `"need at least N points"` → Increase `WINDOW`
- `"numerical instability"` → Reduce ARIMA p/q parameters
- `"no 'value' field"` → Check Prometheus adapter configuration

## Implementation Details

### Model Interface

Both models implement:

```go
type Model interface {
    Train(ctx context.Context, history FeatureFrame) error
    Predict(ctx context.Context, features FeatureFrame) (Forecast, error)
    Name() string
}
```

**Training:** Optional for Baseline (learns patterns if available), Required for ARIMA

**Prediction:** Returns forecast with:
- `Metric`: string
- `Values`: []float64 (length = horizon/step)
- `StepSec`: int
- `Horizon`: int

### Feature Engineering

The forecaster automatically enriches data with time features:

```go
Input (from Prometheus):
  {value: 123.45, timestamp: 1234567890}

Output (features):
  {
    value: 123.45,
    timestamp: 1234567890,
    hour: 14,        // 0-23
    minute: 30,      // 0-59
    dayOfWeek: 2,    // 0-6 (Sunday=0)
  }
```

These time features enable seasonality learning.

## Future Models

Planned for future releases:

- **Prophet**: Facebook's forecasting model with holiday effects
- **Ensemble**: Combine multiple models with weighted voting
- **ML-based**: Neural networks for very complex patterns
- **BYOM (Bring Your Own Model)**: HTTP endpoint contract

## Contributing

To add a new model:

1. Implement the `Model` interface in `pkg/models/`
2. Add constructor in `cmd/forecaster/models/model.go`
3. Add configuration flags in `cmd/forecaster/config/config.go`
4. Write comprehensive tests
5. Create documentation in `docs/models/your-model.md`
6. Update this README with comparison table

See [CONTRIBUTING.md](../../CONTRIBUTING.md) for details.

---

## Additional Resources

- [Architecture Overview](../architecture.md)
- [Capacity Planning](../capacity-planning.md)
- [Prometheus Adapter Configuration](../adapters/prometheus.md)
- [Quick Start Guide](../quickstart.md)

---

**Need help choosing?** Start with **Baseline** and only switch to ARIMA if you need multi-day patterns!
