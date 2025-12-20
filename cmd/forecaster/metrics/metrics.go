// Package metrics provides Prometheus metrics instrumentation for the forecaster.
//
// It exposes operational metrics about the forecaster's pipeline performance,
// including the duration of each stage (collect, predict, capacity planning),
// the age and state of forecasts, and error tracking. All metrics are exposed
// via the /metrics HTTP endpoint for Prometheus scraping.
//
// Metrics exposed:
//   - kedastral_adapter_collect_seconds: Histogram of metric collection duration
//   - kedastral_model_predict_seconds: Histogram of forecast prediction duration
//   - kedastral_capacity_compute_seconds: Histogram of capacity planning duration
//   - kedastral_forecast_age_seconds: Gauge of current forecast age
//   - kedastral_desired_replicas: Gauge of current desired replica count
//   - kedastral_predicted_value: Gauge of current predicted metric value
//   - kedastral_errors_total: Counter of errors by component and reason
//
// All metrics include the workload label for multi-workload deployments.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for the forecaster.
type Metrics struct {
	AdapterCollectSeconds  prometheus.Histogram
	ModelPredictSeconds    prometheus.Histogram
	CapacityComputeSeconds prometheus.Histogram
	ForecastAgeSeconds     prometheus.Gauge
	DesiredReplicas        prometheus.Gauge
	PredictedValue         prometheus.Gauge
	ErrorsTotal            *prometheus.CounterVec
}

// New creates and registers all Prometheus metrics.
func New(workload string) *Metrics {
	return &Metrics{
		AdapterCollectSeconds: promauto.NewHistogram(prometheus.HistogramOpts{
			Name: "kedastral_adapter_collect_seconds",
			Help: "Time spent collecting metrics from adapter",
			ConstLabels: prometheus.Labels{
				"adapter":  "prometheus",
				"workload": workload,
			},
			Buckets: prometheus.DefBuckets, // Default buckets: .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10
		}),

		ModelPredictSeconds: promauto.NewHistogram(prometheus.HistogramOpts{
			Name: "kedastral_model_predict_seconds",
			Help: "Time spent predicting forecast",
			ConstLabels: prometheus.Labels{
				"model":    "baseline",
				"workload": workload,
			},
			Buckets: prometheus.DefBuckets,
		}),

		CapacityComputeSeconds: promauto.NewHistogram(prometheus.HistogramOpts{
			Name: "kedastral_capacity_compute_seconds",
			Help: "Time spent computing desired replicas",
			ConstLabels: prometheus.Labels{
				"workload": workload,
			},
			Buckets: prometheus.DefBuckets,
		}),

		ForecastAgeSeconds: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "kedastral_forecast_age_seconds",
			Help: "Age of the current forecast in seconds",
			ConstLabels: prometheus.Labels{
				"workload": workload,
			},
		}),

		DesiredReplicas: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "kedastral_desired_replicas",
			Help: "Current desired replica count",
			ConstLabels: prometheus.Labels{
				"workload": workload,
			},
		}),

		PredictedValue: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "kedastral_predicted_value",
			Help: "Current predicted metric value (e.g., RPS)",
			ConstLabels: prometheus.Labels{
				"workload": workload,
			},
		}),

		ErrorsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "kedastral_errors_total",
			Help: "Total number of errors by component and reason",
			ConstLabels: prometheus.Labels{
				"workload": workload,
			},
		}, []string{"component", "reason"}),
	}
}

// RecordCollect records the time spent collecting metrics.
func (m *Metrics) RecordCollect(seconds float64) {
	m.AdapterCollectSeconds.Observe(seconds)
}

// RecordPredict records the time spent predicting.
func (m *Metrics) RecordPredict(seconds float64) {
	m.ModelPredictSeconds.Observe(seconds)
}

// RecordCapacity records the time spent computing capacity.
func (m *Metrics) RecordCapacity(seconds float64) {
	m.CapacityComputeSeconds.Observe(seconds)
}

// SetForecastAge sets the current forecast age.
func (m *Metrics) SetForecastAge(seconds float64) {
	m.ForecastAgeSeconds.Set(seconds)
}

// SetDesiredReplicas sets the current desired replica count.
func (m *Metrics) SetDesiredReplicas(replicas int) {
	m.DesiredReplicas.Set(float64(replicas))
}

// SetPredictedValue sets the current predicted metric value.
func (m *Metrics) SetPredictedValue(value float64) {
	m.PredictedValue.Set(value)
}

// RecordError increments the error counter.
func (m *Metrics) RecordError(component, reason string) {
	m.ErrorsTotal.WithLabelValues(component, reason).Inc()
}
