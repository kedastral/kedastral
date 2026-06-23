# Backtesting

`cmd/backtest` replays a historical metric series through a Kedastral forecasting model
and the capacity planner, then reports how accurate the forecasts were and how well the
resulting replica plan matched actual demand. Use it offline to compare models and tune
policy before deploying.

## Build

```bash
make backtest        # -> bin/backtest
```

## Input

A two-column CSV of `timestamp,value`, ordered by time at a fixed step. The timestamp
may be RFC3339 or a Unix timestamp in seconds. A header row is detected and skipped
automatically.

```csv
timestamp,value
2026-01-01T00:00:00Z,210.4
2026-01-01T00:01:00Z,214.9
```

## Run

```bash
bin/backtest -input history.csv \
  -model sarima -sarima-s=60 \
  -step=1m -horizon=30m -window=6h -stride=2h \
  -target-per-pod=50 -min=2 -max=20
```

Key flags (durations use Go syntax extended with `d` (days) and `w` (weeks) — e.g.
`90s`, `30m`, `6h`, `7d`, `1w`):

| Flag | Meaning |
|------|---------|
| `-input` | CSV path (reads stdin if omitted) |
| `-model` | `baseline`, `arima`, `sarima`, or `byom` (+ `-byom-url`) |
| `-step` | Series spacing / forecast resolution |
| `-horizon` | How far ahead each forecast predicts |
| `-window` | Trailing history each model trains on |
| `-stride` | How far the evaluation point advances each iteration (default: step) |
| `-target-per-pod`, `-headroom`, `-min`, `-max`, `-quantile-level` | Capacity policy |
| `-output` | `text` (default) or `json` |

ARIMA/SARIMA orders are configurable with `-arima-*` and `-sarima-*`. For seasonal
models, `-window` should be at least the seasonal period (`-sarima-s` steps); otherwise
every window is skipped.

## Output

```
Backtest report
  model:                  sarima(1,1,1)(1,1,1,60)
  windows evaluated:      33 (skipped 0)
  forecast points:        990
  forecast accuracy:
    MAE:                  10.6932
    RMSE:                 13.4507
    MAPE:                 6.82%
  capacity outcome:
    under-provisioned:    0.10% of steps
    mean over-provision:  23.40%
```

## Metrics

**Forecast accuracy** (predicted vs actual, across all walk-forward windows):
- **MAE** — mean absolute error.
- **RMSE** — root mean squared error (penalizes large misses).
- **MAPE** — mean absolute percentage error; zero-valued actuals are skipped.

**Capacity outcome** (planned replicas vs the bare requirement
`ceil(actual / target-per-pod)`, floored at `-min`):
- **under-provisioned** — fraction of steps where the plan ran fewer replicas than
  required. This is the risk metric; lower is safer.
- **mean over-provision** — average fraction of replicas run beyond the requirement.
  This is the cost metric; some over-provisioning is expected from headroom.

Lower MAPE with low under-provisioning indicates a model that both predicts well and
scales safely. Comparing `baseline` against `arima`/`sarima` on the same series is the
quickest way to choose a model for a workload.
