// Package v1alpha1 contains the Kedastral API types for the kedastral.io group.
//
// It defines the ForecastPolicy and DataSource custom resources reconciled by the
// forecaster's embedded controller. ForecastPolicy describes a workload to forecast
// and scale; DataSource describes a metrics backend (Prometheus, VictoriaMetrics, HTTP)
// that policies reference.
//
// +kubebuilder:object:generate=true
// +groupName=kedastral.io
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

// GroupVersion is the group and version for the kedastral.io API.
var GroupVersion = schema.GroupVersion{Group: "kedastral.io", Version: "v1alpha1"}

// SchemeBuilder registers the kedastral.io types with a runtime scheme.
var SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

// AddToScheme adds the kedastral.io types to a runtime scheme.
var AddToScheme = SchemeBuilder.AddToScheme

func init() {
	SchemeBuilder.Register(
		&ForecastPolicy{}, &ForecastPolicyList{},
		&DataSource{}, &DataSourceList{},
	)
}
