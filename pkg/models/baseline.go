package models

import (
	"context"
	"fmt"
	"math"
)

// BaselineModel implements a predictive forecasting model combining:
//   - Linear trend detection (slope from recent history)
//   - Momentum detection (acceleration/deceleration)
//   - Multi-level seasonality (minute-of-hour and hour-of-day patterns)
//
// **Recommended Usage:**
//   - Training data: 3-24 hours of historical metrics (optimal: 6-12 hours)
//   - Forecast horizon: up to 60 minutes ahead
//   - Works best with: recurring patterns (e.g., traffic spikes, daily cycles)
//
// **Limitations:**
//   - For patterns > 24 hours (weekly cycles), consider ARIMA model
//   - Requires consistent step size in training data
//   - Pattern detection needs at least 2-3 occurrences to learn reliably
//
// Algorithm:
//  1. Training: Learn minute-of-hour and hour-of-day statistical patterns
//  2. Trend: Compute recent slope via linear regression over trailing window
//  3. Momentum: Detect acceleration by comparing recent vs older slopes
//  4. Forecast: For each future step t:
//     a. Base = current + slope*t + 0.5*acceleration*t²
//     b. Seasonal adjustment from learned patterns
//     c. Combine base trend with seasonal component (adaptive weighting)
//  5. Clamp to non-negative values
//
type BaselineModel struct {
	// metric is the name of the metric being forecast
	metric string

	// stepSec is the interval in seconds between forecast points
	stepSec int

	// horizon is the total forecast window in seconds
	horizon int

	// minuteSeasonality stores minute-of-hour patterns (0-59)
	// Captures intra-hour patterns like 30-min spikes
	minuteSeasonality map[int]*seasonalPattern

	// hourSeasonality stores hour-of-day patterns (0-23)
	// Captures daily patterns like business hours vs night
	hourSeasonality map[int]*seasonalPattern

	// residualStdDev is the standard deviation of forecast errors
	// Used to compute quantile predictions for uncertainty estimation
	residualStdDev float64
}

// seasonalPattern holds statistical summary for a recurring pattern
type seasonalPattern struct {
	mean   float64 // average value at this time point
	max    float64 // maximum observed value
	min    float64 // minimum observed value
	count  int     // number of observations
	stddev float64 // standard deviation of values
}

// NewBaselineModel creates a new baseline forecasting model.
func NewBaselineModel(metric string, stepSec, horizon int) *BaselineModel {
	return &BaselineModel{
		metric:            metric,
		stepSec:           stepSec,
		horizon:           horizon,
		minuteSeasonality: make(map[int]*seasonalPattern),
		hourSeasonality:   make(map[int]*seasonalPattern),
	}
}

// Name returns the model identifier.
func (m *BaselineModel) Name() string {
	return "baseline"
}

// Train learns seasonal patterns from historical data.
//
// The model extracts:
//  - Minute-of-hour patterns (0-59): for intra-hour cycles
//  - Hour-of-day patterns (0-23): for daily cycles
//
// For each time bucket, computes: mean, min, max, count
// Requires at least 2 observations per bucket to establish a pattern.
func (m *BaselineModel) Train(ctx context.Context, history FeatureFrame) error {
	if len(history.Rows) == 0 {
		return nil
	}

	minuteValues := make(map[int][]float64)

	hourValues := make(map[int][]float64)

	for _, row := range history.Rows {
		value, hasValue := row["value"]
		if !hasValue {
			continue
		}

		if minute, hasMinute := row["minute"]; hasMinute {
			m := int(minute)
			if m >= 0 && m < 60 {
				minuteValues[m] = append(minuteValues[m], value)
			}
		}

		if hour, hasHour := row["hour"]; hasHour {
			h := int(hour)
			if h >= 0 && h < 24 {
				hourValues[h] = append(hourValues[h], value)
			}
		}
	}

	for minute := 0; minute < 60; minute++ {
		values := minuteValues[minute]
		if len(values) >= 2 {
			m.minuteSeasonality[minute] = computeSeasonalPattern(values)
		}
	}

	for hour := 0; hour < 24; hour++ {
		values := hourValues[hour]
		if len(values) >= 2 {
			m.hourSeasonality[hour] = computeSeasonalPattern(values)
		}
	}

	// Estimate overall forecast uncertainty from seasonal variation
	// Compute average standard deviation across all seasonal patterns
	totalStdDev := 0.0
	patternCount := 0

	for _, pattern := range m.minuteSeasonality {
		if pattern != nil && pattern.stddev > 0 {
			totalStdDev += pattern.stddev
			patternCount++
		}
	}

	for _, pattern := range m.hourSeasonality {
		if pattern != nil && pattern.stddev > 0 {
			totalStdDev += pattern.stddev
			patternCount++
		}
	}

	if patternCount > 0 {
		m.residualStdDev = totalStdDev / float64(patternCount)
	} else {
		// Fallback: compute overall stddev from all values
		allValues := []float64{}
		for _, vals := range minuteValues {
			allValues = append(allValues, vals...)
		}
		for _, vals := range hourValues {
			allValues = append(allValues, vals...)
		}
		if pattern := computeSeasonalPattern(allValues); pattern != nil {
			m.residualStdDev = pattern.stddev
		}
	}

	return nil
}

// computeSeasonalPattern calculates statistical summary from a set of values
func computeSeasonalPattern(values []float64) *seasonalPattern {
	if len(values) == 0 {
		return nil
	}

	sum := 0.0
	min := values[0]
	max := values[0]

	for _, v := range values {
		sum += v
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}

	mean := sum / float64(len(values))

	// Compute standard deviation
	variance := 0.0
	for _, v := range values {
		diff := v - mean
		variance += diff * diff
	}
	stddev := 0.0
	if len(values) > 1 {
		variance = variance / float64(len(values)-1)
		stddev = math.Sqrt(variance)
	}

	return &seasonalPattern{
		mean:   mean,
		min:    min,
		max:    max,
		count:  len(values),
		stddev: stddev,
	}
}

// Predict generates a forecast combining trend, momentum, and seasonality.
//
// Required features:
//   - "value": the metric value (required)
//   - "minute": minute of hour 0-59 (recommended for intra-hour patterns)
//   - "hour": hour of day 0-23 (recommended for daily patterns)
//   - "timestamp": Unix timestamp (optional, for ordering)
//
// Algorithm:
//  1. Detect linear trend (slope) from recent values
//  2. Detect momentum (acceleration) by comparing recent vs older trend
//  3. For each forecast step:
//     - Compute base prediction using trend + momentum
//     - Look up seasonal pattern for that future time
//     - Combine base and seasonal predictions with adaptive weighting
//  4. Clamp to non-negative values
//
// Returns a Forecast with Values of length horizon/stepSec.
func (m *BaselineModel) Predict(ctx context.Context, features FeatureFrame) (Forecast, error) {
	if len(features.Rows) == 0 {
		return Forecast{}, fmt.Errorf("features cannot be empty")
	}

	// Extract value series
	values := make([]float64, 0, len(features.Rows))
	for _, row := range features.Rows {
		if v, ok := row["value"]; ok {
			values = append(values, v)
		}
	}

	if len(values) == 0 {
		return Forecast{}, fmt.Errorf("no 'value' field found in features")
	}

	currentValue := values[len(values)-1]

	// Detect trend (slope) from recent history
	trend := detectTrend(values)

	// Detect momentum (acceleration) by comparing recent vs older trends
	momentum := detectMomentum(values)

	// Get current time context
	currentMinute := -1
	currentHour := -1
	if len(features.Rows) > 0 {
		lastRow := features.Rows[len(features.Rows)-1]
		if m, ok := lastRow["minute"]; ok {
			currentMinute = int(m)
		}
		if h, ok := lastRow["hour"]; ok {
			currentHour = int(h)
		}
	}

	// Generate forecast
	numSteps := m.horizon / m.stepSec
	if numSteps <= 0 {
		numSteps = 1
	}

	forecastValues := make([]float64, numSteps)

	for i := 0; i < numSteps; i++ {
		// Time offset in seconds
		timeOffset := float64((i + 1) * m.stepSec)

		// Base prediction using trend + momentum (quadratic extrapolation)
		// Formula: y(t) = y0 + trend*t + 0.5*momentum*t²
		basePrediction := currentValue + trend*timeOffset + 0.5*momentum*timeOffset*timeOffset/60.0

		// Calculate future time bucket for seasonal lookup
		secondsAhead := (i + 1) * m.stepSec
		minutesAhead := secondsAhead / 60
		hoursAhead := secondsAhead / 3600

		var seasonalValue float64
		var hasSeasonalPattern bool

		// Prefer minute-of-hour seasonality (more granular)
		if currentMinute >= 0 && len(m.minuteSeasonality) > 0 {
			futureMinute := (currentMinute + minutesAhead) % 60
			if pattern, ok := m.minuteSeasonality[futureMinute]; ok && pattern != nil {
				// Use the mean, but favor max if we're detecting upward momentum
				seasonalValue = pattern.mean
				if momentum > 0 && pattern.max > pattern.mean {
					// Blend mean and max based on momentum strength
					seasonalValue = 0.7*pattern.mean + 0.3*pattern.max
				}
				hasSeasonalPattern = true
			}
		}

		// Fall back to hour-of-day seasonality if no minute pattern
		if !hasSeasonalPattern && currentHour >= 0 && len(m.hourSeasonality) > 0 {
			futureHour := (currentHour + hoursAhead) % 24
			if pattern, ok := m.hourSeasonality[futureHour]; ok && pattern != nil {
				seasonalValue = pattern.mean
				if momentum > 0 && pattern.max > pattern.mean {
					seasonalValue = 0.7*pattern.mean + 0.3*pattern.max
				}
				hasSeasonalPattern = true
			}
		}

		var finalValue float64
		if hasSeasonalPattern {
			// Adaptive weighting: trust seasonal pattern more when:
			// - It's significantly different from base prediction (indicates real pattern)
			// - The pattern has been observed multiple times (higher count)

			// If seasonal value is much higher than base, it might indicate an upcoming spike
			ratio := seasonalValue / (basePrediction + 1.0) // +1 to avoid division by zero

			if ratio > 1.5 {
				// Strong seasonal spike expected - trust it heavily
				finalValue = 0.2*basePrediction + 0.8*seasonalValue
			} else if ratio > 1.2 {
				// Moderate seasonal increase
				finalValue = 0.3*basePrediction + 0.7*seasonalValue
			} else if ratio < 0.8 {
				// Seasonal dip expected
				finalValue = 0.4*basePrediction + 0.6*seasonalValue
			} else {
				// Seasonal and trend agree - blend equally
				finalValue = 0.5*basePrediction + 0.5*seasonalValue
			}
		} else {
			// No seasonal pattern - rely on trend + momentum
			finalValue = basePrediction
		}

		// Clamp to non-negative
		if finalValue < 0 {
			finalValue = 0
		}

		forecastValues[i] = finalValue
	}

	quantiles := make(map[float64][]float64)
	if m.residualStdDev > 0 {
		quantileLevels := map[float64]float64{
			0.50: 0.0,
			0.75: 0.674,
			0.90: 1.282,
			0.95: 1.645,
		}

		for q, z := range quantileLevels {
			qValues := make([]float64, len(forecastValues))
			for i, v := range forecastValues {
				qValues[i] = math.Max(0, v+z*m.residualStdDev)
			}
			quantiles[q] = qValues
		}
	}

	return Forecast{
		Metric:    m.metric,
		Values:    forecastValues,
		StepSec:   m.stepSec,
		Horizon:   m.horizon,
		Quantiles: quantiles,
	}, nil
}

// detectTrend computes the slope (rate of change per second) from recent values.
// Uses simple linear regression on the most recent window of data.
//
// Returns slope in units per second (can be positive, negative, or zero).
func detectTrend(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}

	// Use last 10 points or all available, whichever is smaller
	windowSize := 10
	if len(values) < windowSize {
		windowSize = len(values)
	}

	window := values[len(values)-windowSize:]

	// Simple linear regression: y = a + b*x
	// We want b (slope)
	n := float64(len(window))
	sumX := 0.0
	sumY := 0.0
	sumXY := 0.0
	sumX2 := 0.0

	for i, y := range window {
		x := float64(i)
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	// slope = (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)
	denominator := n*sumX2 - sumX*sumX
	if denominator == 0 {
		return 0
	}

	slope := (n*sumXY - sumX*sumY) / denominator

	// slope is in units per data point
	// Convert to units per second (assuming data points are evenly spaced)
	// This is a rough approximation - caller should scale by actual time intervals
	return slope / 60.0 // Assume ~1 minute between points
}

// detectMomentum computes acceleration by comparing recent trend to older trend.
// Returns change in slope (second derivative approximation).
//
// Positive momentum = accelerating upward
// Negative momentum = decelerating or accelerating downward
func detectMomentum(values []float64) float64 {
	if len(values) < 6 {
		return 0 // Need enough data to compare trends
	}

	// Split into two halves: older and recent
	mid := len(values) / 2
	olderValues := values[:mid]
	recentValues := values[mid:]

	// Compute trend for each half
	olderTrend := detectTrend(olderValues)
	recentTrend := detectTrend(recentValues)

	// Momentum = change in trend
	momentum := recentTrend - olderTrend

	return momentum
}
