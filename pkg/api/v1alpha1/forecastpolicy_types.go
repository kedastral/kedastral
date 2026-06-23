package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ScaleTargetRef identifies the workload that the generated KEDA ScaledObject scales.
type ScaleTargetRef struct {
	// APIVersion of the scale target. Defaults to apps/v1.
	// +kubebuilder:default="apps/v1"
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`

	// Kind of the scale target. Defaults to Deployment.
	// +kubebuilder:default="Deployment"
	// +optional
	Kind string `json:"kind,omitempty"`

	// Name of the scale target.
	Name string `json:"name"`
}

// DataSourceRef references a DataSource in the same namespace.
type DataSourceRef struct {
	// Name of the referenced DataSource.
	Name string `json:"name"`
}

// ARIMAParams configures the ARIMA model. Zero values request automatic selection.
type ARIMAParams struct {
	// +optional
	P int `json:"p,omitempty"`
	// +optional
	D int `json:"d,omitempty"`
	// +optional
	Q int `json:"q,omitempty"`
}

// SARIMAParams configures the seasonal ARIMA model.
type SARIMAParams struct {
	// +optional
	P int `json:"p,omitempty"`
	// +optional
	D int `json:"d,omitempty"`
	// +optional
	Q int `json:"q,omitempty"`
	// +optional
	SeasonalP int `json:"seasonalP,omitempty"`
	// +optional
	SeasonalD int `json:"seasonalD,omitempty"`
	// +optional
	SeasonalQ int `json:"seasonalQ,omitempty"`
	// SeasonalPeriod is the seasonal cycle length (e.g. 24 for hourly with a daily pattern).
	// +optional
	SeasonalPeriod int `json:"seasonalPeriod,omitempty"`
}

// ModelSpec selects and configures the forecasting model.
type ModelSpec struct {
	// Type is the forecasting model: baseline, arima, sarima, or byom.
	// +kubebuilder:validation:Enum=baseline;arima;sarima;byom
	// +kubebuilder:default=baseline
	Type string `json:"type"`

	// +optional
	ARIMA *ARIMAParams `json:"arima,omitempty"`

	// +optional
	SARIMA *SARIMAParams `json:"sarima,omitempty"`

	// BYOMURL is the bring-your-own-model service URL. Required when type is byom.
	// +optional
	BYOMURL string `json:"byomURL,omitempty"`
}

// ForecastSpec controls the forecast horizon and cadence. Durations use Go format (30s, 5m, 1h).
type ForecastSpec struct {
	// +kubebuilder:default="30m"
	// +optional
	Horizon string `json:"horizon,omitempty"`
	// +kubebuilder:default="1m"
	// +optional
	Step string `json:"step,omitempty"`
	// +kubebuilder:default="30s"
	// +optional
	Interval string `json:"interval,omitempty"`
	// +kubebuilder:default="30m"
	// +optional
	Window string `json:"window,omitempty"`
}

// CapacitySpec configures the capacity planner that converts forecasts to replicas.
type CapacitySpec struct {
	// TargetPerPod is the target metric value handled by a single pod.
	TargetPerPod float64 `json:"targetPerPod"`

	// Headroom is a safety multiplier applied when quantiles are unavailable.
	// +kubebuilder:default=1.2
	// +optional
	Headroom float64 `json:"headroom,omitempty"`

	// QuantileLevel selects quantile-based planning (e.g. p90, p95, or 0.90). "0" disables it.
	// +kubebuilder:default="0"
	// +optional
	QuantileLevel string `json:"quantileLevel,omitempty"`

	// +kubebuilder:default=1
	// +optional
	MinReplicas int `json:"minReplicas,omitempty"`

	// +kubebuilder:default=100
	// +optional
	MaxReplicas int `json:"maxReplicas,omitempty"`

	// UpMaxFactorPerStep limits scale-up per forecast step.
	// +kubebuilder:default=2
	// +optional
	UpMaxFactorPerStep float64 `json:"upMaxFactorPerStep,omitempty"`

	// DownMaxPercentPerStep limits scale-down per forecast step (0-100).
	// +kubebuilder:default=50
	// +optional
	DownMaxPercentPerStep int `json:"downMaxPercentPerStep,omitempty"`
}

// ForecastPolicySpec defines the desired forecasting and scaling behavior for a workload.
type ForecastPolicySpec struct {
	// ScaleTargetRef is the workload scaled by the generated ScaledObject.
	ScaleTargetRef ScaleTargetRef `json:"scaleTargetRef"`

	// Metric is the metric name used in logs and snapshots.
	Metric string `json:"metric"`

	// DataSourceRef references the DataSource to collect metrics from.
	DataSourceRef DataSourceRef `json:"dataSourceRef"`

	// +optional
	Model ModelSpec `json:"model,omitempty"`

	// +optional
	Forecast ForecastSpec `json:"forecast,omitempty"`

	Capacity CapacitySpec `json:"capacity"`

	// LeadTime is how far ahead the scaler looks for proactive scale-up.
	// Passed to the generated ScaledObject trigger metadata.
	// +kubebuilder:default="10m"
	// +optional
	LeadTime string `json:"leadTime,omitempty"`
}

// ForecastPolicyStatus reports the observed state of a ForecastPolicy.
type ForecastPolicyStatus struct {
	// ObservedGeneration is the generation last processed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastForecastTime is when the most recent forecast snapshot was generated.
	// +optional
	LastForecastTime *metav1.Time `json:"lastForecastTime,omitempty"`

	// CurrentReplicas is the most recently planned replica count.
	// +optional
	CurrentReplicas int `json:"currentReplicas,omitempty"`

	// DesiredReplicas is the peak planned replica count over the forecast horizon.
	// +optional
	DesiredReplicas int `json:"desiredReplicas,omitempty"`

	// ScaledObjectName is the name of the generated KEDA ScaledObject.
	// +optional
	ScaledObjectName string `json:"scaledObjectName,omitempty"`

	// Conditions represent the latest observations of the policy state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=fp
// +kubebuilder:printcolumn:name="Target",type=string,JSONPath=`.spec.scaleTargetRef.name`
// +kubebuilder:printcolumn:name="Model",type=string,JSONPath=`.spec.model.type`
// +kubebuilder:printcolumn:name="Current",type=integer,JSONPath=`.status.currentReplicas`
// +kubebuilder:printcolumn:name="Desired",type=integer,JSONPath=`.status.desiredReplicas`

// ForecastPolicy describes a workload to forecast and scale predictively.
type ForecastPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ForecastPolicySpec   `json:"spec,omitempty"`
	Status ForecastPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ForecastPolicyList contains a list of ForecastPolicy.
type ForecastPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ForecastPolicy `json:"items"`
}
