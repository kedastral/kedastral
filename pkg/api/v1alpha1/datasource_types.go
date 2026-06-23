package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DataSourceSpec describes a metrics backend that ForecastPolicies collect from.
type DataSourceSpec struct {
	// Type is the adapter kind: prometheus, victoriametrics, or http.
	// +kubebuilder:validation:Enum=prometheus;victoriametrics;http
	Type string `json:"type"`

	// Config holds adapter-specific key/value settings (e.g. url, query).
	// Keys map directly to the adapter factory configuration.
	// +optional
	Config map[string]string `json:"config,omitempty"`
}

// DataSourceStatus reports the observed state of a DataSource.
type DataSourceStatus struct {
	// ObservedGeneration is the generation last processed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest observations of the DataSource state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ds
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`

// DataSource describes a metrics backend referenced by ForecastPolicies.
type DataSource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DataSourceSpec   `json:"spec,omitempty"`
	Status DataSourceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DataSourceList contains a list of DataSource.
type DataSourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DataSource `json:"items"`
}
