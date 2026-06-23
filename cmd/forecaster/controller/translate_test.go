package controller

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kedastralv1alpha1 "github.com/HatiCode/kedastral/pkg/api/v1alpha1"
)

func basePolicy() *kedastralv1alpha1.ForecastPolicy {
	return &kedastralv1alpha1.ForecastPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "shop"},
		Spec: kedastralv1alpha1.ForecastPolicySpec{
			ScaleTargetRef: kedastralv1alpha1.ScaleTargetRef{Name: "web"},
			Metric:         "http_rps",
			DataSourceRef:  kedastralv1alpha1.DataSourceRef{Name: "prom"},
			Capacity: kedastralv1alpha1.CapacitySpec{
				TargetPerPod:          100,
				Headroom:              1.2,
				MinReplicas:           2,
				MaxReplicas:           20,
				UpMaxFactorPerStep:    2,
				DownMaxPercentPerStep: 50,
			},
		},
	}
}

func promDataSource() *kedastralv1alpha1.DataSource {
	return &kedastralv1alpha1.DataSource{
		ObjectMeta: metav1.ObjectMeta{Name: "prom", Namespace: "shop"},
		Spec: kedastralv1alpha1.DataSourceSpec{
			Type:   "prometheus",
			Config: map[string]string{"url": "http://prom:9090", "query": "sum(rate(x[1m]))"},
		},
	}
}

func TestWorkloadKey(t *testing.T) {
	if got := workloadKey("shop", "web"); got != "shop-web" {
		t.Errorf("workloadKey = %q, want %q", got, "shop-web")
	}
}

func TestToWorkloadConfig_Defaults(t *testing.T) {
	wc, err := toWorkloadConfig(basePolicy(), promDataSource())
	if err != nil {
		t.Fatalf("toWorkloadConfig() error = %v", err)
	}

	if wc.Name != "shop-web" {
		t.Errorf("Name = %q, want shop-web", wc.Name)
	}
	if wc.Adapter != "prometheus" {
		t.Errorf("Adapter = %q, want prometheus", wc.Adapter)
	}
	if wc.AdapterConfig["query"] != "sum(rate(x[1m]))" {
		t.Errorf("AdapterConfig query not propagated: %v", wc.AdapterConfig)
	}
	if wc.Model != "baseline" {
		t.Errorf("Model = %q, want baseline (default)", wc.Model)
	}
	if wc.Horizon != 30*time.Minute {
		t.Errorf("Horizon = %v, want 30m default", wc.Horizon)
	}
	if wc.Step != time.Minute {
		t.Errorf("Step = %v, want 1m default", wc.Step)
	}
	if wc.QuantileLevel != "0" {
		t.Errorf("QuantileLevel = %q, want 0 default", wc.QuantileLevel)
	}
}

func TestToWorkloadConfig_ARIMA(t *testing.T) {
	policy := basePolicy()
	policy.Spec.Model = kedastralv1alpha1.ModelSpec{
		Type:  "arima",
		ARIMA: &kedastralv1alpha1.ARIMAParams{P: 2, D: 1, Q: 1},
	}
	policy.Spec.Forecast = kedastralv1alpha1.ForecastSpec{Horizon: "1h", Step: "5m", Interval: "1m", Window: "2h"}

	wc, err := toWorkloadConfig(policy, promDataSource())
	if err != nil {
		t.Fatalf("toWorkloadConfig() error = %v", err)
	}

	if wc.Model != "arima" {
		t.Errorf("Model = %q, want arima", wc.Model)
	}
	if wc.ARIMA_P != 2 || wc.ARIMA_D != 1 || wc.ARIMA_Q != 1 {
		t.Errorf("ARIMA params = (%d,%d,%d), want (2,1,1)", wc.ARIMA_P, wc.ARIMA_D, wc.ARIMA_Q)
	}
	if wc.Horizon != time.Hour {
		t.Errorf("Horizon = %v, want 1h", wc.Horizon)
	}
}

func TestToWorkloadConfig_InvalidDuration(t *testing.T) {
	policy := basePolicy()
	policy.Spec.Forecast.Horizon = "not-a-duration"

	if _, err := toWorkloadConfig(policy, promDataSource()); err == nil {
		t.Fatal("expected error for invalid duration, got nil")
	}
}

func TestToWorkloadConfig_FailsValidation(t *testing.T) {
	policy := basePolicy()
	policy.Spec.Capacity.TargetPerPod = 0 // invalid: must be > 0

	if _, err := toWorkloadConfig(policy, promDataSource()); err == nil {
		t.Fatal("expected validation error for zero targetPerPod, got nil")
	}
}
