package backtest

import (
	"math"
	"testing"
)

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-6
}

func TestMAE(t *testing.T) {
	if got := MAE([]float64{1, 2, 3}, []float64{1, 4, 3}); !approxEqual(got, 2.0/3.0) {
		t.Errorf("MAE = %v, want %v", got, 2.0/3.0)
	}
	if got := MAE(nil, nil); got != 0 {
		t.Errorf("MAE(empty) = %v, want 0", got)
	}
}

func TestRMSE(t *testing.T) {
	got := RMSE([]float64{1, 2, 3}, []float64{1, 4, 3})
	want := math.Sqrt(4.0 / 3.0)
	if !approxEqual(got, want) {
		t.Errorf("RMSE = %v, want %v", got, want)
	}
}

func TestMAPE(t *testing.T) {
	if got := MAPE([]float64{110, 90}, []float64{100, 100}); !approxEqual(got, 10) {
		t.Errorf("MAPE = %v, want 10", got)
	}
	// Zero actuals are skipped; here only the second point counts.
	if got := MAPE([]float64{50, 90}, []float64{0, 100}); !approxEqual(got, 10) {
		t.Errorf("MAPE with zero actual = %v, want 10", got)
	}
	if got := MAPE([]float64{1}, []float64{0}); got != 0 {
		t.Errorf("MAPE all-zero actuals = %v, want 0", got)
	}
}

func TestUnderProvisionedRate(t *testing.T) {
	got := UnderProvisionedRate([]int{1, 2, 3}, []int{2, 2, 2})
	if !approxEqual(got, 1.0/3.0) {
		t.Errorf("UnderProvisionedRate = %v, want %v", got, 1.0/3.0)
	}
}

func TestMeanOverProvision(t *testing.T) {
	got := MeanOverProvision([]int{3, 2}, []int{2, 2})
	if !approxEqual(got, 0.25) {
		t.Errorf("MeanOverProvision = %v, want 0.25", got)
	}
	// needed <= 0 entries are ignored.
	if got := MeanOverProvision([]int{5}, []int{0}); got != 0 {
		t.Errorf("MeanOverProvision needed=0 = %v, want 0", got)
	}
}
