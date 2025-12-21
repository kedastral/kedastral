// Package capacity converts forecasted load into desired replica counts
// using a deterministic policy (target per pod, headroom, clamps).
package capacity

import (
	"math"
)

// Policy defines how forecasted load is translated into replicas.
type Policy struct {
	// TargetPerPod is the sustainable throughput per pod at your SLO (e.g., RPS per pod).
	// Must be > 0.
	TargetPerPod float64

	// Headroom is a multiplicative safety factor (e.g., 1.2 for +20%).
	// Used as fallback when quantiles are not available.
	// Must be >= 1.0
	Headroom float64

	// QuantileLevel specifies which quantile to use for capacity planning (e.g., 0.90 for p90).
	// When > 0 and quantiles are provided, uses the quantile directly instead of headroom.
	// Set to 0 to always use headroom (default behavior).
	// Common values: 0.75 (moderate), 0.90 (conservative), 0.95 (very conservative)
	QuantileLevel float64

	// Min/Max replica bounds. MaxReplicas == 0 means "no upper bound".
	MinReplicas int
	MaxReplicas int

	// UpMaxFactorPerStep caps how fast we can scale up relative to the previous step.
	// Example: 2.0 allows doubling per step at most. If <= 0, defaults to 2.0.
	UpMaxFactorPerStep float64

	// DownMaxPercentPerStep caps how fast we can scale down (percentage of previous).
	// Example: 50 means we can drop at most 50% from the previous step. Clamped to [0,100].
	DownMaxPercentPerStep int

	// PrewarmWindowSteps defines how many steps ahead to consider when computing replicas.
	// 0 = single point at i (conservative). N>0 = max over [i .. i+N] (aggressive).
	PrewarmWindowSteps int

	// RoundingMode controls how fractional pods are turned into integers.
	// "ceil" (default), "round", or "floor".
	RoundingMode string
}

// ToReplicas converts a forecasted load series into desired replicas, applying the policy.
// prev is the previously applied desired replica count (from the last control loop tick).
// forecast contains the metric values for each future step (e.g., RPS).
// stepSec is the step resolution in seconds.
// quantiles optionally provides quantile forecasts. When provided and Policy.QuantileLevel > 0,
// uses the specified quantile instead of applying headroom to the point forecast.
func ToReplicas(prev int, forecast []float64, stepSec int, p Policy, quantiles map[float64][]float64) []int {
	if len(forecast) == 0 {
		return nil
	}
	if p.TargetPerPod <= 0 {
		p.TargetPerPod = 1
	}
	if p.Headroom < 1 {
		p.Headroom = 1
	}
	if p.MinReplicas < 0 {
		p.MinReplicas = 0
	}
	if p.MaxReplicas > 0 && p.MaxReplicas < p.MinReplicas {
		p.MaxReplicas = p.MinReplicas
	}
	if stepSec <= 0 {
		stepSec = 60
	}
	if p.UpMaxFactorPerStep <= 0 {
		p.UpMaxFactorPerStep = 2.0
	}
	if p.DownMaxPercentPerStep < 0 {
		p.DownMaxPercentPerStep = 0
	}
	if p.DownMaxPercentPerStep > 100 {
		p.DownMaxPercentPerStep = 100
	}
	if p.PrewarmWindowSteps < 0 {
		p.PrewarmWindowSteps = 0
	}

	useQuantile := p.QuantileLevel > 0 && quantiles != nil && len(quantiles[p.QuantileLevel]) == len(forecast)

	adj := make([]float64, len(forecast))
	for i, v := range forecast {
		if v < 0 {
			v = 0
		}

		var capacityValue float64
		if useQuantile {
			capacityValue = quantiles[p.QuantileLevel][i]
			if capacityValue < 0 {
				capacityValue = 0
			}
		} else {
			capacityValue = v * p.Headroom
		}

		raw := capacityValue / p.TargetPerPod
		adj[i] = raw
	}

	res := make([]int, len(forecast))
	prevOut := clampBounds(prev, p.MinReplicas, p.MaxReplicas)

	for i := range forecast {
		jStart := i
		jEnd := i + p.PrewarmWindowSteps
		if jEnd >= len(adj) {
			jEnd = len(adj) - 1
		}
		need := 0.0
		for j := jStart; j <= jEnd; j++ {
			if adj[j] > need {
				need = adj[j]
			}
		}

		desired := roundPods(need, p.RoundingMode)

		// Apply bounds, then change clamps, then bounds again.
		desired = clampBounds(desired, p.MinReplicas, p.MaxReplicas)
		desired = clampChange(prevOut, desired, p.UpMaxFactorPerStep, p.DownMaxPercentPerStep)
		desired = clampBounds(desired, p.MinReplicas, p.MaxReplicas)

		res[i] = desired
		prevOut = desired
	}
	return res
}

func roundPods(x float64, mode string) int {
	switch mode {
	case "floor":
		return int(math.Floor(x))
	case "round":
		return int(math.Round(x))
	default: // "ceil" or anything else
		return int(math.Ceil(x))
	}
}

func clampBounds(x, lo, hi int) int {
	if hi > 0 && x > hi {
		return hi
	}
	if x < lo {
		return lo
	}
	return x
}

func clampChange(prev, next int, upFactor float64, downPct int) int {
	if prev < 0 {
		prev = 0
	}
	// When we don't have prior capacity, allow the requested value directly,
	// but still guard absurd ups with upFactor if provided.
	if prev == 0 {
		if upFactor > 0 {
			maxUp := int(math.Ceil(float64(1) * upFactor))
			if next > maxUp {
				return maxUp
			}
		}
		return next
	}
	maxUp := int(math.Ceil(float64(prev) * upFactor))
	minDown := int(math.Floor(float64(prev) * (1.0 - float64(downPct)/100.0)))
	if next > maxUp {
		return maxUp
	}
	if next < minDown {
		return minDown
	}
	return next
}
