# Quantile Forecasting

Quantile forecasting provides uncertainty-aware capacity planning by predicting multiple quantiles of future load instead of a single point estimate.

## Overview

### Traditional Approach (Headroom)
```
Point Forecast → × Headroom (1.2) → Capacity
Example: 100 RPS → × 1.2 → 120 RPS capacity needed
```

**Problem**: Fixed headroom doesn't account for actual forecast uncertainty.

### Quantile Approach
```
Forecast → [p50, p75, p90, p95] → Use p90 → Capacity
Example: [100 (p50), 115 (p75), 130 (p90), 145 (p95)] → 130 RPS capacity
```

**Benefit**: Uncertainty naturally captured; no fixed headroom needed.

## How It Works

### 1. Model Training
Both baseline and ARIMA models now compute forecast uncertainty:

**Baseline Model:**
- Computes standard deviation of seasonal patterns
- Uses historical variability to estimate uncertainty

**ARIMA Model:**
- Computes standard deviation of residuals
- Uses forecast errors to estimate uncertainty
- Uncertainty grows with forecast horizon (√t)

### 2. Quantile Generation
Models automatically generate quantiles using standard normal distribution:

```go
Quantiles: map[float64][]float64{
    0.50: []float64{100, 105, 110},  // median (p50)
    0.75: []float64{113, 118, 124},  // 75th percentile
    0.90: []float64{128, 134, 141},  // 90th percentile (conservative)
    0.95: []float64{137, 144, 152},  // 95th percentile (very conservative)
}
```

**Z-scores used:**
- p50: 0.0 (median = point forecast)
- p75: 0.674
- p90: 1.282
- p95: 1.645

### 3. Capacity Planning
When `Policy.QuantileLevel` is set, the planner uses the specified quantile directly:

```go
// Without quantiles (traditional)
capacity = forecast * headroom

// With quantiles
capacity = quantiles[p90]  // No headroom multiplication
```

## Configuration

### Enable Quantile-Based Scaling

Add `quantileLevel` to your forecaster config:

```yaml
# Forecaster flags (p-notation - recommended)
-quantile-level=p90   # Use p90 for capacity planning
-headroom=1.2         # Used as fallback if quantiles unavailable

# Alternative: decimal notation
-quantile-level=0.90  # Also accepted
```

**Recommended quantile levels:**
- `p75` (or `0.75`): Moderate risk tolerance (75% confident)
- `p90` (or `0.90`): Conservative (90% confident) - **recommended default**
- `p95` (or `0.95`): Very conservative (95% confident)

### Disable Quantile-Based Scaling

Set `quantileLevel=0` to always use headroom:

```yaml
-quantile-level=0  # Disabled, use headroom
-headroom=1.2      # Always applied
```

## Comparison: Headroom vs Quantiles

### Scenario: Variable Load Pattern

```
Actual Load: [100, 150, 80, 200, 120]
Forecast (p50): [100, 145, 85, 195, 115]
Forecast StdDev: 20
```

**With Headroom (1.2):**
```
Capacity = [120, 174, 102, 234, 138]
Fixed 20% buffer regardless of uncertainty
```

**With Quantiles (p90):**
```
Capacity = [126, 171, 111, 221, 141]
Adaptive buffer based on actual variance
```

**Benefit**: Quantiles provide tighter capacity during stable periods, more buffer during volatile periods.

## Implementation Details

### Forecast Structure

```go
type Forecast struct {
    Metric    string
    Values    []float64  // Point forecast (mean)
    StepSec   int
    Horizon   int
    Quantiles map[float64][]float64  // Optional quantile predictions
}
```

### Capacity Policy

```go
type Policy struct {
    TargetPerPod   float64
    Headroom       float64  // Fallback when quantiles unavailable
    QuantileLevel  float64  // 0.90 = use p90, 0 = disabled
    MinReplicas    int
    MaxReplicas    int
    // ... other fields
}
```

### Storage Snapshot

Snapshots now include quantiles for transparency:

```json
{
  "workload": "api-app",
  "metric": "rps",
  "values": [100, 105, 110],
  "quantiles": {
    "0.50": [100, 105, 110],
    "0.90": [128, 134, 141],
    "0.95": [137, 144, 152]
  },
  "desiredReplicas": [3, 3, 3]
}
```

## Migration Guide

### Existing Deployments

Quantile support is **backward compatible**:

1. **No changes required** - models automatically compute quantiles
2. **Headroom still works** - set `quantileLevel=0` or omit the flag
3. **Gradual adoption** - enable per-workload

### Recommended Migration

1. **Test with same capacity:**
   ```bash
   # Find equivalent quantile level
   # If headroom=1.2, try p75 or p90
   -quantile-level=p90 -headroom=1.2
   ```

2. **Monitor for 24-48 hours:**
   - Check if pods scale appropriately
   - Compare p90 capacity vs headroom capacity
   - Verify no SLO violations

3. **Tune quantile level:**
   ```bash
   # More conservative
   -quantile-level=p95

   # More aggressive
   -quantile-level=p75
   ```

## Advantages

✅ **Uncertainty-aware**: Captures forecast confidence
✅ **Adaptive**: Buffer scales with variability
✅ **Risk-based**: Choose quantile based on risk tolerance
✅ **Transparent**: Quantiles visible in API responses
✅ **Backward compatible**: Headroom still available as fallback

## Limitations

⚠️ **Assumes normal distribution**: Z-scores work best for normally distributed errors
⚠️ **Requires training data**: Needs sufficient history to estimate uncertainty
⚠️ **Not for all patterns**: Complex multi-modal distributions may need custom models

## Future Enhancements

- **Conformal prediction intervals**: Distribution-free uncertainty
- **Multi-model ensembles**: Combine baseline + ARIMA quantiles
- **Adaptive quantile selection**: Automatically tune based on SLO violations
- **Time-varying uncertainty**: Different quantiles for different times of day

## References

- Z-scores: https://en.wikipedia.org/wiki/Standard_score
- Quantile regression: https://en.wikipedia.org/wiki/Quantile_regression
- Forecast intervals: https://otexts.com/fpp3/prediction-intervals.html
