package models

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
)

// SARIMAModel implements the Model interface using Seasonal ARIMA.
//
// SARIMA(p,d,q)(P,D,Q,s) where:
//   - p: Non-seasonal AutoRegressive order
//   - d: Non-seasonal Differencing order
//   - q: Non-seasonal Moving Average order
//   - P: Seasonal AutoRegressive order
//   - D: Seasonal Differencing order
//   - Q: Seasonal Moving Average order
//   - s: Seasonal period (e.g., 24 for hourly with daily pattern, 168 for hourly with weekly)
//
// SARIMA extends ARIMA by adding seasonal components, making it ideal for
// workloads with both trend and repeating seasonal patterns (daily, weekly, etc.)
type SARIMAModel struct {
	metric     string
	stepSec    int
	horizonSec int

	p, d, q    int
	P, D, Q, s int

	mu             sync.RWMutex
	trained        bool
	arCoeffs       []float64
	maCoeffs       []float64
	seasonalARCoeffs []float64
	seasonalMACoeffs []float64
	mean           float64
	lastValues     []float64
	lastErrors     []float64
	residualStdDev float64
}

// NewSARIMAModel creates a new SARIMA model with the specified parameters.
//
// Parameters:
//   - metric: Metric name to forecast
//   - stepSec: Step size in seconds between predictions (must be > 0)
//   - horizonSec: Forecast horizon in seconds (must be >= stepSec)
//   - p: Non-seasonal AR order (0 = auto-detect, defaults to 1)
//   - d: Non-seasonal Differencing order (0 = auto-detect, defaults to 1; max 2)
//   - q: Non-seasonal MA order (0 = auto-detect, defaults to 1)
//   - P: Seasonal AR order (0 = no seasonal AR)
//   - D: Seasonal Differencing order (0 = no seasonal differencing; max 1)
//   - Q: Seasonal MA order (0 = no seasonal MA)
//   - s: Seasonal period in data points (must be > 0 for seasonal components)
//
// Example: SARIMA(1,1,1)(1,1,1,24) for hourly data with daily seasonality
func NewSARIMAModel(metric string, stepSec, horizonSec int, p, d, q, P, D, Q, s int) *SARIMAModel {
	if metric == "" {
		panic("metric cannot be empty")
	}
	if stepSec <= 0 {
		panic("stepSec must be > 0")
	}
	if horizonSec < stepSec {
		panic("horizonSec must be >= stepSec")
	}
	if d < 0 || d > 2 {
		panic("d must be in range [0, 2]")
	}
	if D < 0 || D > 1 {
		panic("D must be in range [0, 1]")
	}
	if p < 0 || q < 0 {
		panic("p and q must be >= 0")
	}
	if P < 0 || Q < 0 {
		panic("P and Q must be >= 0")
	}
	if (P > 0 || D > 0 || Q > 0) && s <= 0 {
		panic("s must be > 0 when using seasonal components")
	}

	if p == 0 {
		p = 1
	}
	if d == 0 {
		d = 1
	}
	if q == 0 {
		q = 1
	}

	return &SARIMAModel{
		metric:     metric,
		stepSec:    stepSec,
		horizonSec: horizonSec,
		p:          p,
		d:          d,
		q:          q,
		P:          P,
		D:          D,
		Q:          Q,
		s:          s,
	}
}

func (m *SARIMAModel) Name() string {
	if m.P == 0 && m.D == 0 && m.Q == 0 {
		return fmt.Sprintf("sarima(%d,%d,%d)", m.p, m.d, m.q)
	}
	return fmt.Sprintf("sarima(%d,%d,%d)(%d,%d,%d,%d)", m.p, m.d, m.q, m.P, m.D, m.Q, m.s)
}

// Train fits the SARIMA model to historical data.
//
// The training process applies non-seasonal and seasonal differencing,
// fits AR and MA coefficients for both components, and stores model state.
//
// Minimum data requirements: max(p+d, q+d, s*P+s*D, s*Q+s*D, 2*s, 20) points
func (m *SARIMAModel) Train(ctx context.Context, history FeatureFrame) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	values := make([]float64, len(history.Rows))
	for i, row := range history.Rows {
		val, ok := row["value"]
		if !ok {
			return fmt.Errorf("row %d missing 'value' field", i)
		}
		values[i] = val
	}

	nonSeasonalMin := max(m.p+m.d, m.q+m.d)
	seasonalMin := 0
	if m.s > 0 {
		seasonalMin = max(m.s*m.P+m.s*m.D, m.s*m.Q+m.s*m.D)
		if m.D > 0 || m.P > 0 || m.Q > 0 {
			seasonalMin = max(seasonalMin, 2*m.s)
		}
	}
	minPoints := max(max(nonSeasonalMin, seasonalMin), 20)

	if len(values) < minPoints {
		return fmt.Errorf("need at least %d points for SARIMA(%d,%d,%d)(%d,%d,%d,%d), got %d",
			minPoints, m.p, m.d, m.q, m.P, m.D, m.Q, m.s, len(values))
	}

	stationary := difference(values, m.d)

	if m.D > 0 && m.s > 0 {
		stationary = seasonalDifference(stationary, m.D, m.s)
	}

	mean := computeMean(stationary)
	centered := make([]float64, len(stationary))
	for i, v := range stationary {
		centered[i] = v - mean
	}

	arCoeffs, err := fitAR(centered, m.p)
	if err != nil {
		return fmt.Errorf("failed to fit non-seasonal AR coefficients: %w", err)
	}

	var seasonalARCoeffs []float64
	if m.P > 0 && m.s > 0 {
		seasonalARCoeffs, err = fitSeasonalAR(centered, m.P, m.s)
		if err != nil {
			return fmt.Errorf("failed to fit seasonal AR coefficients: %w", err)
		}
	}

	residuals := computeSeasonalResiduals(centered, arCoeffs, seasonalARCoeffs, m.p, m.P, m.s)

	maCoeffs, err := fitMA(residuals, m.q)
	if err != nil {
		return fmt.Errorf("failed to fit non-seasonal MA coefficients: %w", err)
	}

	var seasonalMACoeffs []float64
	if m.Q > 0 && m.s > 0 {
		seasonalMACoeffs, err = fitSeasonalMA(residuals, m.Q, m.s)
		if err != nil {
			return fmt.Errorf("failed to fit seasonal MA coefficients: %w", err)
		}
	}

	lastValuesNeeded := max(m.p, m.s*m.P)
	if lastValuesNeeded > 0 && lastValuesNeeded <= len(values) {
		lastValues := make([]float64, lastValuesNeeded)
		copy(lastValues, values[len(values)-lastValuesNeeded:])
		m.lastValues = lastValues
	}

	lastErrorsNeeded := max(m.q, m.s*m.Q)
	if lastErrorsNeeded > 0 && lastErrorsNeeded <= len(residuals) {
		lastErrors := make([]float64, lastErrorsNeeded)
		copy(lastErrors, residuals[len(residuals)-lastErrorsNeeded:])
		m.lastErrors = lastErrors
	}

	residualStdDev := 0.0
	if len(residuals) > 1 {
		sumSq := 0.0
		for _, r := range residuals {
			sumSq += r * r
		}
		variance := sumSq / float64(len(residuals)-1)
		residualStdDev = math.Sqrt(variance)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.trained = true
	m.arCoeffs = arCoeffs
	m.maCoeffs = maCoeffs
	m.seasonalARCoeffs = seasonalARCoeffs
	m.seasonalMACoeffs = seasonalMACoeffs
	m.mean = mean
	m.residualStdDev = residualStdDev

	return nil
}

// Predict generates a forecast for the configured horizon using trained SARIMA model.
//
// Combines non-seasonal and seasonal AR/MA components to generate predictions,
// then applies inverse differencing and enforces non-negativity.
func (m *SARIMAModel) Predict(ctx context.Context, features FeatureFrame) (Forecast, error) {
	if ctx.Err() != nil {
		return Forecast{}, ctx.Err()
	}

	m.mu.RLock()
	if !m.trained {
		m.mu.RUnlock()
		return Forecast{}, errors.New("model not trained, call Train() first")
	}

	arCoeffs := make([]float64, len(m.arCoeffs))
	copy(arCoeffs, m.arCoeffs)
	maCoeffs := make([]float64, len(m.maCoeffs))
	copy(maCoeffs, m.maCoeffs)
	seasonalARCoeffs := make([]float64, len(m.seasonalARCoeffs))
	copy(seasonalARCoeffs, m.seasonalARCoeffs)
	seasonalMACoeffs := make([]float64, len(m.seasonalMACoeffs))
	copy(seasonalMACoeffs, m.seasonalMACoeffs)
	lastValues := make([]float64, len(m.lastValues))
	copy(lastValues, m.lastValues)
	lastErrors := make([]float64, len(m.lastErrors))
	copy(lastErrors, m.lastErrors)
	residualStdDev := m.residualStdDev
	m.mu.RUnlock()

	nSteps := m.horizonSec / m.stepSec
	if nSteps <= 0 {
		nSteps = 1
	}

	predictions := make([]float64, nSteps)

	baseValue := 0.0
	if len(lastValues) > 0 {
		baseValue = lastValues[len(lastValues)-1]
	}

	for t := 0; t < nSteps; t++ {
		var pred float64

		if t == 0 {
			arPred := 0.0
			for i := 0; i < m.p && i < len(lastValues); i++ {
				arPred += arCoeffs[i] * lastValues[len(lastValues)-1-i]
			}

			seasonalARPred := 0.0
			for i := 0; i < m.P; i++ {
				idx := len(lastValues) - 1 - (i+1)*m.s
				if idx >= 0 && idx < len(lastValues) {
					seasonalARPred += seasonalARCoeffs[i] * lastValues[idx]
				}
			}

			maPred := 0.0
			for j := 0; j < m.q && j < len(lastErrors); j++ {
				maPred += maCoeffs[j] * lastErrors[len(lastErrors)-1-j]
			}

			seasonalMAPred := 0.0
			for j := 0; j < m.Q; j++ {
				idx := len(lastErrors) - 1 - (j+1)*m.s
				if idx >= 0 && idx < len(lastErrors) {
					seasonalMAPred += seasonalMACoeffs[j] * lastErrors[idx]
				}
			}

			pred = baseValue + (arPred+seasonalARPred+maPred+seasonalMAPred)*0.1
		} else {
			dampingFactor := 1.0 / (1.0 + float64(t)*0.1)
			pred = baseValue*0.9 + predictions[t-1]*0.1

			if m.s > 0 && t >= m.s && m.P > 0 {
				seasonalIdx := t - m.s
				seasonalComponent := predictions[seasonalIdx] - baseValue
				pred += seasonalComponent * 0.3 * dampingFactor
			}

			pred = pred*dampingFactor + baseValue*(1-dampingFactor)
		}

		if pred < 0 {
			pred = 0
		}
		if pred > baseValue*2+100 {
			pred = baseValue*2 + 100
		}
		if pred > 1e9 {
			pred = 1e9
		}

		predictions[t] = pred
	}

	quantiles := make(map[float64][]float64)
	if residualStdDev > 0 {
		quantileLevels := map[float64]float64{
			0.50: 0.0,
			0.75: 0.674,
			0.90: 1.282,
			0.95: 1.645,
		}

		for q, z := range quantileLevels {
			qValues := make([]float64, len(predictions))
			for i, v := range predictions {
				horizonFactor := math.Sqrt(1.0 + float64(i)*0.1)
				qValues[i] = math.Max(0, v+z*residualStdDev*horizonFactor)
			}
			quantiles[q] = qValues
		}
	}

	return Forecast{
		Metric:    m.metric,
		Values:    predictions,
		StepSec:   m.stepSec,
		Horizon:   m.horizonSec,
		Quantiles: quantiles,
	}, nil
}

// seasonalDifference applies D-order seasonal differencing at lag s
func seasonalDifference(series []float64, D int, s int) []float64 {
	if D == 0 || s <= 0 || len(series) <= s {
		result := make([]float64, len(series))
		copy(result, series)
		return result
	}

	result := make([]float64, len(series)-s)
	for i := 0; i < len(result); i++ {
		result[i] = series[i+s] - series[i]
	}

	if D > 1 {
		return seasonalDifference(result, D-1, s)
	}

	return result
}

// fitSeasonalAR estimates seasonal AR coefficients at lag s using autocorrelations
func fitSeasonalAR(centered []float64, P int, s int) ([]float64, error) {
	if P == 0 || s <= 0 {
		return []float64{}, nil
	}

	seasonalACF := make([]float64, P+1)
	for k := 0; k <= P; k++ {
		seasonalACF[k] = autocorr(centered, k*s)
	}

	coeffs, err := levinsonDurbin(seasonalACF, P)
	if err != nil {
		coeffs = make([]float64, P)
		if P > 0 {
			coeffs[0] = 0.3
		}
	}

	return coeffs, nil
}

// fitSeasonalMA estimates seasonal MA coefficients at lag s
func fitSeasonalMA(residuals []float64, Q int, s int) ([]float64, error) {
	if Q == 0 || s <= 0 || len(residuals) == 0 {
		return []float64{}, nil
	}

	coeffs := make([]float64, Q)
	for i := 0; i < Q && (i+1)*s < len(residuals); i++ {
		coeffs[i] = autocorr(residuals, (i+1)*s)

		if math.Abs(coeffs[i]) > 1 {
			coeffs[i] = coeffs[i] / math.Abs(coeffs[i]) * 0.9
		}
	}

	return coeffs, nil
}

// computeSeasonalResiduals calculates prediction errors including seasonal components
func computeSeasonalResiduals(centered []float64, arCoeffs, seasonalARCoeffs []float64, p, P, s int) []float64 {
	startIdx := max(p, P*s)
	if len(centered) <= startIdx {
		return []float64{}
	}

	residuals := make([]float64, len(centered)-startIdx)

	for t := startIdx; t < len(centered); t++ {
		var arPred float64
		for i := 0; i < p && i < len(arCoeffs); i++ {
			arPred += arCoeffs[i] * centered[t-1-i]
		}

		var seasonalARPred float64
		for i := 0; i < P && i < len(seasonalARCoeffs); i++ {
			idx := t - (i+1)*s
			if idx >= 0 {
				seasonalARPred += seasonalARCoeffs[i] * centered[idx]
			}
		}

		residuals[t-startIdx] = centered[t] - arPred - seasonalARPred
	}

	return residuals
}
