# 🔧 Kedastral Tuning Guide

This guide explains **how to tune Kedastral’s capacity planner** so you get the best balance between **SLO protection** and **cost**. It’s written for platform/SRE teams and assumes you’re using the **planner math** described in `docs/math.md`.

---

## TL;DR Defaults (Safe Starters)

Use these until real data suggests otherwise:

| Setting | Default | Why |
|---|---:|---|
| `TargetPerPod` | workload-specific | Throughput each pod sustains at SLO (measure!) |
| `Headroom` | `1.2` | 20% buffer for forecast error and jitter |
| `PrewarmWindowSteps` | `0` | Conservative, predictable capacity calculation |
| `RoundingMode` | `ceil` | Bias against under-provisioning |
| `UpMaxFactorPerStep` | `1.5–2.0` | Avoid cold-start stampedes |
| `DownMaxPercentPerStep` | `25–50` | Avoid cache flush / thrash |
| `MinReplicas` | `≥ 2` | Keep warm pool, reduce cold starts |
| `MaxReplicas` | workload-specific | Cost guardrail & dependency protection |

**Note:** Lead-time is configured on the **scaler** (not the forecaster). See scaler configuration for `--lead-time` setting.

Keep **reactive** KEDA triggers (e.g., CPU/RPS) enabled: KEDA takes the **max** across triggers, giving you a safe fallback.

---

## What Each Knob Does

### `TargetPerPod` (required)
Throughput one pod can handle at your SLO (e.g., **RPS/pod** at p95 latency target).
**How to set:**
1. Load-test or use production telemetry during steady periods.
2. Take the sustainable value, not the absolute peak.
3. Re-evaluate after major code/runtime changes.

**Symptoms if wrong:**
- Too low → always over-provisioned (cost).
- Too high → under-provisioning during ramps (SLO impact).

---

### `Headroom` (default `1.2`)
Safety multiplier applied before rounding. Absorbs forecast error, GC spikes, and noisy neighbors.

- Increase if p95/p99 spikes at ramps or during GC.
- Decrease if you’re consistently over-provisioned and reactive triggers never fire.

**Rules of thumb:**
- API with modest variance: `1.1–1.2`
- Bursty traffic or heavy GC/JIT: `1.2–1.35`

---

### `PrewarmWindowSteps` (default `0`)
When > 0, capacity planner takes the **max** over `[i .. i+window]` for each forecast position. This can help catch upcoming spikes within the forecast window.

- Keep `0` for most services (predictable behavior).
- Use `1` only for **known, sharp, fixed-time bursts** (e.g., live event starts).
- Document when enabled; it can surprise operators.

---

## Scaler Configuration

### `--lead-time` (scaler flag, default `5m`)
**Note:** This is configured on the **scaler**, not the forecaster.

How far ahead the scaler looks in the forecast for proactive scaling. Compensates for pod startup time (image pull, JIT, warming caches).

- If pods take 45s to become effective → choose `5m-10m` lead-time
- Increase if you still see cold-start penalties
- Decrease if you consistently scale too early

**How it works:**
1. Scaler takes MAX over DesiredReplicas[0...leadSteps] from forecast
2. Returns this value to KEDA
3. KEDA takes MAX across ALL scalers (predictive + reactive triggers)

**Scaling behavior:**
- **Scale-up:** Proactive (scales 10min before predicted spike via lead-time window MAX)
- **Scale-down:** Gradual (limited by DownMaxPercentPerStep clamps)
- **Safety:** Reactive triggers (CPU/RPS) act as fallback when predictions are wrong

---

### `RoundingMode` (default `ceil`)
Controls how fractional pods are made integer.

- `ceil` (recommended): safer SLO behavior.
- `round`: slightly cheaper, riskier near thresholds.
- `floor`: generally not recommended.

---

### `UpMaxFactorPerStep` (default `1.5–2.0`)
Limit for how fast replicas can increase per step.

- Lower (e.g., `1.5`) if fast ramps cause cold-start storms or dependency stress.
- Higher (e.g., `2.5`) if your pods come online very quickly and SLOs are tight.

**Sanity checks:**
- With prev=10 and `1.5`, next step can be at most 15.
- With prev=2 and `2.0`, next step can be at most 4.

---

### `DownMaxPercentPerStep` (default `25–50`)
Limit for how fast replicas can decrease per step, as a percentage of the previous count.

- Lower (e.g., `25`) for cache-heavy services (keep them warm).
- Higher (e.g., `60–80`) for stateless/shallow-cache services.

**Example:** prev=20, `50%` → next step ≥ `floor(20 * 0.5) = 10`.

---

### `MinReplicas` / `MaxReplicas`
- **Min:** Protects against cold starts and keeps a warm buffer (`≥ 2` recommended).
- **Max:** Protects your wallet and downstream systems. Must be realistic; KEDA will still be limited by this bound.

---

## Tuning Playbook (Practical Steps)

1) **Shadow Mode (observe only)**
   - Run Kedastral without enforcing scaling; log/metrics only.
   - Track deltas between predictive desired and current replicas.

2) **Canary Enablement**
   - Enable for one critical service or a small fraction of replicas.
   - Keep reactive HPA triggers in place.

3) **Measure & Adjust (1–2 weeks)**
   - Watch:
     - `under_provision_minutes` (p95 > SLO while utilization ~100%)
     - `wasted_replica_minutes` (utilization ≪ target)
     - `time_to_recover` after spikes
     - cold-start/error spikes
   - **If under-provisioning:** increase `Headroom` or `LeadTimeSeconds`; consider `UpMaxFactorPerStep` ↑.
   - **If wasteful:** decrease `Headroom`; consider `DownMaxPercentPerStep` ↑ or `LeadTimeSeconds` ↓.

4) **Lock Defaults per Workload**
   - Document chosen values & rationale.
   - Add alert thresholds for future regression.

---

## Example Parameter Sets

### A) Steady API (low variance)
```
Headroom:               1.15
LeadTimeSeconds:        60
PrewarmWindowSteps:     0
UpMaxFactorPerStep:     1.5
DownMaxPercentPerStep:  40
Min/MaxReplicas:        2 / 200
```

### B) Bursty Events (known start times)
```
Headroom:               1.25
LeadTimeSeconds:        120
PrewarmWindowSteps:     1
UpMaxFactorPerStep:     2.0
DownMaxPercentPerStep:  30
Min/MaxReplicas:        4 / 500
```

### C) Heavy Cache / JIT Warmup
```
Headroom:               1.3
LeadTimeSeconds:        120
PrewarmWindowSteps:     0
UpMaxFactorPerStep:     1.5
DownMaxPercentPerStep:  25
Min/MaxReplicas:        6 / 300
```

---

## Observability Checklist

Expose and dashboard:
- `kedastral_desired_replicas` (planner output at each step)
- `kedastral_predicted_value` (value used at lead index)
- `kedastral_capacity_target_per_pod`
- `kedastral_headroom`
- `kedastral_lead_steps`
- `kedastral_up/down_clamp_applied` (boolean or delta)
- `forecast_age_seconds`
- Compare **predictive** vs **reactive** (KEDA/HPA) decisions

This makes “why did we scale to N?” obvious during incidents.

---

## FAQ

- **Should I disable CPU/RPS HPA once I enable Kedastral?**
  No. Keep them. KEDA will take the **max**, giving you a safety net.

- **How do I choose `TargetPerPod`?**
  Load test or use production telemetry under steady load at your SLO. Revisit after major releases.

- **We still see cold starts. What first?**
  Increase `LeadTimeSeconds`, then `Headroom`. If still spiky, raise `UpMaxFactorPerStep` cautiously.

- **We’re over-provisioned all the time.**
  Lower `Headroom`, consider smaller lead time, and allow faster downscaling (`DownMaxPercentPerStep` ↑).

---

## Next Steps

- Keep this guide close to the planner config (e.g., link from README).
- Once you add Helm/CRDs, mirror these settings into values with sensible defaults.
