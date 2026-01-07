# SARIMA Model

## Overview

The **SARIMA Model** (Seasonal AutoRegressive Integrated Moving Average) is Kedastral's most advanced forecasting algorithm for workloads with **both trends and repeating seasonal patterns**. It extends ARIMA by adding seasonal components, making it ideal for predictable daily, weekly, or monthly cycles.

**SARIMA(p,d,q)(P,D,Q,s)** where:
- **p** = Non-seasonal AutoRegressive order
- **d** = Non-seasonal Differencing order (trend removal)
- **q** = Non-seasonal Moving Average order
- **P** = Seasonal AutoRegressive order
- **D** = Seasonal Differencing order
- **Q** = Seasonal Moving Average order
- **s** = Seasonal period (e.g., 24 for hourly data with daily pattern)

## When to Use SARIMA Model

✅ **Use SARIMA if you have:**
- Strong repeating patterns (daily rush hours, weekday/weekend differences)
- Hourly/5-minute data with daily patterns (s=24 or s=288)
- Daily data with weekly patterns (s=7)
- Combination of trend AND seasonality
- At least 2 full seasonal periods of historical data
- Need for statistical seasonal modeling

❌ **Use ARIMA instead if:**
- Trend but NO clear seasonal pattern
- Patterns are irregular or unpredictable
- Limited training data (< 2 seasonal periods)

❌ **Use Baseline instead if:**
- Simple hourly patterns (baseline has basic hour-of-day support)
- Zero-configuration setup needed
- Minimal memory/CPU available

## How It Works

### Algorithm Overview

SARIMA combines **six components** (3 non-seasonal + 3 seasonal):

```
Non-Seasonal Components:
1. AR(p): AutoRegressive
   └─> Predicts from p previous values

2. I(d): Integrated
   └─> Removes trend via differencing

3. MA(q): Moving Average
   └─> Uses q past forecast errors

Seasonal Components:
4. SAR(P): Seasonal AutoRegressive
   └─> Predicts from P seasonal lags (P*s periods back)

5. SI(D): Seasonal Integrated
   └─> Removes seasonal trend via differencing at lag s

6. SMA(Q): Seasonal Moving Average
   └─> Uses Q seasonal errors
```

### Mathematical Formulation

Given a time series `y(t)` with seasonal period `s`:

1. **Seasonal Differencing** (remove seasonal pattern):
   ```
   D=1: y_s(t) = y(t) - y(t-s)
   Example (s=24): today's 3pm - yesterday's 3pm
   ```

2. **Non-Seasonal Differencing** (remove trend):
   ```
   d=1: y'(t) = y_s(t) - y_s(t-1)
   ```

3. **Non-Seasonal AR Component**:
   ```
   AR(p): ŷ(t) = φ₁y(t-1) + ... + φₚy(t-p)
   ```

4. **Seasonal AR Component**:
   ```
   SAR(P): ŷ(t) += Φ₁y(t-s) + Φ₂y(t-2s) + ... + Φₚy(t-Ps)
   Example (P=1, s=24): uses value from 24 hours ago
   ```

5. **Non-Seasonal MA Component**:
   ```
   MA(q): ŷ(t) += θ₁ε(t-1) + ... + θ_qε(t-q)
   ```

6. **Seasonal MA Component**:
   ```
   SMA(Q): ŷ(t) += Θ₁ε(t-s) + ... + Θ_Qε(t-Qs)
   ```

7. **Final prediction**: Combine all components + invert differencing

### Training Process

```
1. Extract historical values
   └─> Minimum: max(p+d, s*P+s*D, 2*s) points required

2. Apply non-seasonal differencing (d times)
   └─> Remove linear/quadratic trends

3. Apply seasonal differencing (D times at lag s)
   └─> Remove seasonal pattern

4. Compute mean of stationary series

5. Fit non-seasonal AR coefficients (φ₁...φₚ)
   └─> Using Yule-Walker equations

6. Fit seasonal AR coefficients (Φ₁...Φₚ) at lag s
   └─> Using seasonal autocorrelations

7. Compute residuals from AR predictions

8. Fit non-seasonal MA coefficients (θ₁...θ_q)
   └─> Using innovations algorithm

9. Fit seasonal MA coefficients (Θ₁...Θ_Q) at lag s

10. Store last values and errors for prediction
```

### Prediction Process

```
1. Start with historical values and errors
2. For each future step:
   ├─> Apply non-seasonal AR (recent values)
   ├─> Apply seasonal AR (values s periods back)
   ├─> Apply non-seasonal MA (recent errors)
   ├─> Apply seasonal MA (errors s periods back)
   ├─> Add mean
   └─> Invert differencing
3. Clamp to non-negative values
4. Generate uncertainty quantiles
```

## Configuration

### Environment Variables / Flags

| Variable | Flag | Default | Description |
|----------|------|---------|-------------|
| `MODEL` | `--model` | `baseline` | Set to `sarima` |
| `SARIMA_P` | `--sarima-p` | `0` (auto→1) | Non-seasonal AR order |
| `SARIMA_D` | `--sarima-d` | `0` (auto→1) | Non-seasonal differencing |
| `SARIMA_Q` | `--sarima-q` | `0` (auto→1) | Non-seasonal MA order |
| `SARIMA_SP` | `--sarima-sp` | `1` | Seasonal AR order (P) |
| `SARIMA_SD` | `--sarima-sd` | `1` | Seasonal differencing (D) |
| `SARIMA_SQ` | `--sarima-sq` | `1` | Seasonal MA order (Q) |
| `SARIMA_S` | `--sarima-s` | `24` | Seasonal period |
| `WINDOW` | `--window` | `30m` | Training window (use ≥2*s) |

### Seasonal Period (s) Selection

**Critical parameter:** Must match your data granularity and pattern!

| Data Granularity | Pattern | s Value | Example |
|-----------------|---------|---------|---------|
| 5-minute samples | Daily | `288` | 24h * 60min / 5min |
| Hourly samples | Daily | `24` | 24 hours |
| Hourly samples | Weekly | `168` | 24h * 7 days |
| Daily samples | Weekly | `7` | 7 days |
| Daily samples | Monthly | `30` | ~30 days |

**Example calculations:**
```bash
# Hourly data with daily pattern
--sarima-s=24

# 5-minute data with daily pattern
--sarima-s=288  # (24 * 60) / 5

# Hourly data with weekly pattern
--sarima-s=168  # 24 * 7

# 15-minute data with daily pattern
--sarima-s=96   # (24 * 60) / 15
```

### Parameter Selection Guide

#### **Start with defaults: SARIMA(1,1,1)(1,1,1,s)**

Most workloads work well with:
- `p=1, d=1, q=1` (non-seasonal)
- `P=1, D=1, Q=1` (seasonal)
- `s` = based on your pattern (see table above)

#### **When to tune parameters:**

**Increase P (Seasonal AR):**
- Strong correlation with multiple past seasonal cycles
- Example: P=2 uses both 24h and 48h ago

**Increase p (Non-Seasonal AR):**
- Recent values strongly influence next value
- Short-term momentum effects

**Set D=0 (No Seasonal Differencing):**
- Seasonal pattern is stable/stationary
- Over-differencing causes instability

**Increase Q/q (MA orders):**
- Forecast errors show autocorrelation
- Model needs error correction

### Recommended Configurations

#### **Hourly Web Traffic (Daily Pattern)**
```bash
--model=sarima
--sarima-p=1 --sarima-d=1 --sarima-q=1
--sarima-sp=1 --sarima-sd=1 --sarima-sq=1
--sarima-s=24
--window=168h  # 1 week of training data
```

#### **Hourly API with Weekend Differences (Weekly Pattern)**
```bash
--model=sarima
--sarima-p=1 --sarima-d=1 --sarima-q=1
--sarima-sp=1 --sarima-sd=1 --sarima-sq=1
--sarima-s=168
--window=336h  # 2 weeks of training data
```

#### **5-Minute High-Frequency Data (Daily Pattern)**
```bash
--model=sarima
--sarima-p=2 --sarima-d=1 --sarima-q=1
--sarima-sp=1 --sarima-sd=1 --sarima-sq=1
--sarima-s=288
--window=48h  # 2 days
```

#### **Batch Jobs (Daily with Trend)**
```bash
--model=sarima
--sarima-p=1 --sarima-d=1 --sarima-q=1
--sarima-sp=1 --sarima-sd=1 --sarima-sq=1
--sarima-s=24
--window=168h
```

## Usage Examples

### Command Line

```bash
# Basic SARIMA with daily pattern
./forecaster \
  --workload=my-api \
  --metric=http_rps \
  --model=sarima \
  --sarima-s=24 \
  --prom-url=http://prometheus:9090 \
  --prom-query='sum(rate(http_requests_total[1m]))' \
  --window=168h

# Advanced: weekly pattern with custom parameters
./forecaster \
  --workload=batch-job \
  --metric=queue_depth \
  --model=sarima \
  --sarima-p=2 --sarima-d=1 --sarima-q=1 \
  --sarima-sp=1 --sarima-sd=1 --sarima-sq=1 \
  --sarima-s=168 \
  --window=336h
```

### Environment Variables

```bash
export MODEL=sarima
export SARIMA_P=1
export SARIMA_D=1
export SARIMA_Q=1
export SARIMA_SP=1
export SARIMA_SD=1
export SARIMA_SQ=1
export SARIMA_S=24
export WINDOW=168h

./forecaster --workload=my-api --metric=http_rps
```

### Kubernetes Deployment

See `examples/deployment-sarima.yaml` for complete example.

```yaml
containers:
- name: forecaster
  image: kedastral/forecaster:latest
  args:
  - --model=sarima
  - --sarima-p=1
  - --sarima-d=1
  - --sarima-q=1
  - --sarima-sp=1
  - --sarima-sd=1
  - --sarima-sq=1
  - --sarima-s=24
  - --window=168h
  resources:
    requests:
      memory: "256Mi"  # SARIMA needs more memory
      cpu: "200m"
```

### Multi-Workload Config File

See `examples/workloads-sarima.yaml` for complete example.

```yaml
workloads:
  - name: web-api
    metric: http_rps
    model: sarima
    sarimaP: 1
    sarimaD: 1
    sarimaQ: 1
    sarimaSP: 1
    sarimaSD: 1
    sarimaSQ: 1
    sarimaS: 24
    window: 168h
    prometheusURL: http://prometheus:9090
    prometheusQuery: sum(rate(http_requests_total[1m]))
```

## Performance Characteristics

| Metric | Value | Notes |
|--------|-------|-------|
| Training time (200 points) | ~10-50ms | Depends on p,d,q,P,D,Q |
| Training time (1000 points) | ~100-500ms | Scales with data size |
| Prediction time (30 steps) | <100ms | Constant time |
| Memory overhead | ~50-100MB | Stores seasonal state |
| Min data points | 2*s | Need 2 seasonal periods |
| Recommended data | 7*s | 1 week for s=24 |

## Comparison: SARIMA vs ARIMA vs Baseline

| Feature | Baseline | ARIMA | SARIMA |
|---------|----------|-------|--------|
| Handles trend | ❌ | ✅ | ✅ |
| Handles seasonality | Basic | ❌ | ✅ |
| Training required | No | Yes | Yes |
| Training time | - | <5s | <10s |
| Memory usage | ~10MB | ~30MB | ~100MB |
| Min data points | 10 | 20 | 2*s (48+) |
| Best for | Stable load | Trending | Seasonal+Trend |
| Configuration | Zero | Medium | High |

## Troubleshooting

### Common Issues

**Problem:** Training fails with "insufficient data"
- **Solution:** Increase `--window` to at least `2*s` periods
- **Example:** For s=24, use `--window=48h` minimum

**Problem:** Predictions are unstable / oscillating
- **Solution:** Reduce seasonal differencing: `--sarima-sd=0`
- **Or:** Decrease P/Q orders

**Problem:** Poor accuracy despite seasonal pattern
- **Solution:** Verify correct seasonal period (s)
- **Check:** s=24 for hourly/daily, s=168 for hourly/weekly
- **Verify:** Data granularity matches s calculation

**Problem:** High memory usage
- **Solution:** Reduce P, Q orders or use shorter window
- **Alternative:** Use ARIMA if seasonality is weak

**Problem:** Slow training
- **Solution:** Reduce p, q, P, Q orders
- **Or:** Use shorter training window

### Validation Checklist

✅ Seasonal period (s) matches data granularity
✅ Training window ≥ 2*s periods
✅ Historical data has clear repeating pattern
✅ Memory limits allow for seasonal state storage
✅ Forecasts align with expected seasonal pattern

## Technical Details

### Implementation

- **Pure Go implementation** (no external ML libraries)
- **Thread-safe** training and prediction
- **Reuses ARIMA components** for non-seasonal parts
- **Levinson-Durbin** algorithm for AR coefficients
- **Innovations algorithm** for MA coefficients
- **Seasonal autocorrelation** for seasonal components

### Data Requirements

Minimum points needed: `max(p+d, s*P+s*D, s*Q+s*D, 2*s, 20)`

**Examples:**
- SARIMA(1,1,1)(1,1,1,24): 48 points minimum (2 days)
- SARIMA(2,1,1)(1,1,1,168): 336 points minimum (2 weeks)
- SARIMA(1,1,1)(1,1,1,288): 576 points minimum (2 days of 5-min data)

### Limitations

- Max seasonal period tested: s=168 (weekly hourly data)
- Max seasonal differencing: D=1
- No automatic parameter selection (manual tuning required)
- Requires stationary or trend-stationary data
- Not suitable for irregular/unpredictable patterns

## References

- [ARIMA Model Documentation](./arima.md) - Non-seasonal version
- [Baseline Model Documentation](./baseline.md) - Simple alternative
- [Examples](../../examples/) - Deployment examples
- Box, G. E. P., Jenkins, G. M., & Reinsel, G. C. (2015). Time Series Analysis: Forecasting and Control

## Next Steps

1. **Start simple:** Use SARIMA(1,1,1)(1,1,1,s) with correct s
2. **Verify seasonality:** Check forecasts align with expected pattern
3. **Monitor accuracy:** Compare predicted vs actual over time
4. **Tune if needed:** Adjust P/Q for stronger seasonality
5. **Scale up:** Deploy to production with adequate resources
