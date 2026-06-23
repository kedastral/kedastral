package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	kedastralv1alpha1 "github.com/HatiCode/kedastral/pkg/api/v1alpha1"
)

// scaledObjectGVK is the KEDA ScaledObject type. It is handled as an unstructured
// object so Kedastral does not depend on the heavy KEDA module; the type only needs
// to be installed in the cluster at runtime.
var scaledObjectGVK = schema.GroupVersionKind{
	Group:   "keda.sh",
	Version: "v1alpha1",
	Kind:    "ScaledObject",
}

// reconcileScaledObject creates or updates the KEDA ScaledObject that wires the
// external scaler to the policy's scale target. The ScaledObject is owned by the
// ForecastPolicy so it is garbage-collected when the policy is deleted. It returns
// the ScaledObject name.
func (r *ForecastPolicyReconciler) reconcileScaledObject(ctx context.Context, policy *kedastralv1alpha1.ForecastPolicy, workload string) (string, error) {
	so := &unstructured.Unstructured{}
	so.SetGroupVersionKind(scaledObjectGVK)
	so.SetNamespace(policy.Namespace)
	so.SetName(policy.Name)

	target := policy.Spec.ScaleTargetRef
	apiVersion := target.APIVersion
	if apiVersion == "" {
		apiVersion = "apps/v1"
	}
	kind := target.Kind
	if kind == "" {
		kind = "Deployment"
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, so, func() error {
		scaleTargetRef := map[string]any{
			"apiVersion": apiVersion,
			"kind":       kind,
			"name":       target.Name,
		}
		if err := unstructured.SetNestedMap(so.Object, scaleTargetRef, "spec", "scaleTargetRef"); err != nil {
			return err
		}

		if err := unstructured.SetNestedField(so.Object, int64(policy.Spec.Capacity.MinReplicas), "spec", "minReplicaCount"); err != nil {
			return err
		}
		if err := unstructured.SetNestedField(so.Object, int64(policy.Spec.Capacity.MaxReplicas), "spec", "maxReplicaCount"); err != nil {
			return err
		}

		trigger := map[string]any{
			"type": "external",
			"metadata": map[string]any{
				"scalerAddress": r.ScalerAddress,
				"workload":      workload,
			},
		}
		if err := unstructured.SetNestedSlice(so.Object, []any{trigger}, "spec", "triggers"); err != nil {
			return err
		}

		return controllerutil.SetControllerReference(policy, so, r.Scheme)
	})
	if err != nil {
		return "", fmt.Errorf("reconcile ScaledObject: %w", err)
	}

	return so.GetName(), nil
}
