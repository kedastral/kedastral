// Package backtest replays a historical metric series through Kedastral's forecasting
// models and capacity planner, reporting forecast-accuracy and capacity-outcome metrics.
//
// It performs a walk-forward evaluation: at each step it trains a fresh model on a
// trailing window, predicts the next horizon, and compares the predictions (and the
// replicas the planner would choose) against the actual values that followed.
package backtest

import "math"

// MAE returns the mean absolute error between predicted and actual values.
// The slices must have equal length; it returns 0 for empty input.
func MAE(predicted, actual []float64) float64 {
	if len(predicted) == 0 {
		return 0
	}
	var sum float64
	for i := range predicted {
		sum += math.Abs(predicted[i] - actual[i])
	}
	return sum / float64(len(predicted))
}

// RMSE returns the root mean squared error between predicted and actual values.
func RMSE(predicted, actual []float64) float64 {
	if len(predicted) == 0 {
		return 0
	}
	var sum float64
	for i := range predicted {
		diff := predicted[i] - actual[i]
		sum += diff * diff
	}
	return math.Sqrt(sum / float64(len(predicted)))
}

// MAPE returns the mean absolute percentage error (0-100) between predicted and actual
// values. Points where the actual value is zero are skipped to avoid division by zero.
func MAPE(predicted, actual []float64) float64 {
	var sum float64
	var count int
	for i := range predicted {
		if actual[i] == 0 {
			continue
		}
		sum += math.Abs((predicted[i] - actual[i]) / actual[i])
		count++
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count) * 100
}

// UnderProvisionedRate returns the fraction of steps (0-1) where the planned replica
// count was below the actual requirement. This is the key risk metric: high values
// mean the forecast scaled too late or too low.
func UnderProvisionedRate(planned, needed []int) float64 {
	if len(planned) == 0 {
		return 0
	}
	var under int
	for i := range planned {
		if planned[i] < needed[i] {
			under++
		}
	}
	return float64(under) / float64(len(planned))
}

// MeanOverProvision returns the average fractional over-provisioning across steps:
// mean of max(0, (planned-needed)/needed) over steps where needed > 0. A value of 0.2
// means the plan ran ~20% more replicas than strictly required on average.
func MeanOverProvision(planned, needed []int) float64 {
	var sum float64
	var count int
	for i := range planned {
		if needed[i] <= 0 {
			continue
		}
		over := float64(planned[i]-needed[i]) / float64(needed[i])
		if over > 0 {
			sum += over
		}
		count++
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}
