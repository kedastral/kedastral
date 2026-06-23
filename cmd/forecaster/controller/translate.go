package controller

import (
	"time"

	"github.com/HatiCode/kedastral/cmd/forecaster/config"
	kedastralv1alpha1 "github.com/HatiCode/kedastral/pkg/api/v1alpha1"
	"github.com/HatiCode/kedastral/pkg/durationx"
)

// workloadKey derives the store/forecaster key for a ForecastPolicy. It includes the
// namespace so policies with the same name in different namespaces do not collide in
// the shared snapshot store. The result is also used as the ScaledObject trigger's
// workload metadata so the scaler queries the matching snapshot.
func workloadKey(namespace, name string) string {
	return namespace + "-" + name
}

// parseDurationOr parses a Go duration string, falling back to a default when empty.
func parseDurationOr(value string, fallback time.Duration) (time.Duration, error) {
	if value == "" {
		return fallback, nil
	}
	return durationx.Parse(value)
}

// toWorkloadConfig translates a ForecastPolicy and its referenced DataSource into the
// internal WorkloadConfig used by the forecast pipeline. The returned config is
// validated and normalized with the same defaults as flag mode.
func toWorkloadConfig(policy *kedastralv1alpha1.ForecastPolicy, ds *kedastralv1alpha1.DataSource) (config.WorkloadConfig, error) {
	horizon, err := parseDurationOr(policy.Spec.Forecast.Horizon, 30*time.Minute)
	if err != nil {
		return config.WorkloadConfig{}, err
	}
	step, err := parseDurationOr(policy.Spec.Forecast.Step, time.Minute)
	if err != nil {
		return config.WorkloadConfig{}, err
	}
	interval, err := parseDurationOr(policy.Spec.Forecast.Interval, 30*time.Second)
	if err != nil {
		return config.WorkloadConfig{}, err
	}
	window, err := parseDurationOr(policy.Spec.Forecast.Window, 30*time.Minute)
	if err != nil {
		return config.WorkloadConfig{}, err
	}

	model := policy.Spec.Model.Type
	if model == "" {
		model = "baseline"
	}

	quantileLevel := policy.Spec.Capacity.QuantileLevel
	if quantileLevel == "" {
		quantileLevel = "0"
	}

	wc := config.WorkloadConfig{
		Name:                  workloadKey(policy.Namespace, policy.Name),
		Metric:                policy.Spec.Metric,
		Adapter:               ds.Spec.Type,
		AdapterConfig:         ds.Spec.Config,
		Horizon:               horizon,
		Step:                  step,
		Interval:              interval,
		Window:                window,
		Model:                 model,
		TargetPerPod:          policy.Spec.Capacity.TargetPerPod,
		Headroom:              policy.Spec.Capacity.Headroom,
		QuantileLevel:         quantileLevel,
		MinReplicas:           policy.Spec.Capacity.MinReplicas,
		MaxReplicas:           policy.Spec.Capacity.MaxReplicas,
		UpMaxFactorPerStep:    policy.Spec.Capacity.UpMaxFactorPerStep,
		DownMaxPercentPerStep: policy.Spec.Capacity.DownMaxPercentPerStep,
		BYOMURL:               policy.Spec.Model.BYOMURL,
	}

	if policy.Spec.Model.ARIMA != nil {
		wc.ARIMA_P = policy.Spec.Model.ARIMA.P
		wc.ARIMA_D = policy.Spec.Model.ARIMA.D
		wc.ARIMA_Q = policy.Spec.Model.ARIMA.Q
	}

	if policy.Spec.Model.SARIMA != nil {
		wc.SARIMA_P = policy.Spec.Model.SARIMA.P
		wc.SARIMA_D = policy.Spec.Model.SARIMA.D
		wc.SARIMA_Q = policy.Spec.Model.SARIMA.Q
		wc.SARIMA_SP = policy.Spec.Model.SARIMA.SeasonalP
		wc.SARIMA_SD = policy.Spec.Model.SARIMA.SeasonalD
		wc.SARIMA_SQ = policy.Spec.Model.SARIMA.SeasonalQ
		wc.SARIMA_S = policy.Spec.Model.SARIMA.SeasonalPeriod
	}

	if err := config.ValidateWorkload(&wc); err != nil {
		return config.WorkloadConfig{}, err
	}

	return wc, nil
}
