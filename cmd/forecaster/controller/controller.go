// Package controller implements the embedded Kedastral operator. It reconciles
// ForecastPolicy and DataSource custom resources into running in-process forecast
// loops and generates a KEDA ScaledObject per policy.
//
// The controller drives forecast loops through the ForecasterManager interface,
// which the forecaster's main package implements by wrapping its dynamic
// MultiForecaster. This keeps the reconciler decoupled from the forecast pipeline
// and unit-testable with a fake manager.
package controller

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/HatiCode/kedastral/cmd/forecaster/config"
	kedastralv1alpha1 "github.com/HatiCode/kedastral/pkg/api/v1alpha1"
	"github.com/HatiCode/kedastral/pkg/storage"
)

const failureRequeueInterval = 30 * time.Second

// ForecasterManager runs and stops in-process forecast loops keyed by workload name.
type ForecasterManager interface {
	// Upsert starts or replaces the forecast loop for wc.Name.
	Upsert(ctx context.Context, wc config.WorkloadConfig) error
	// Remove stops and unregisters the forecast loop for the given workload key.
	Remove(name string)
}

// ForecastPolicyReconciler reconciles ForecastPolicy resources.
type ForecastPolicyReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Manager       ForecasterManager
	Store         storage.Store
	ScalerAddress string
	Logger        *slog.Logger
}

// Reconcile drives a ForecastPolicy towards its desired state: a running forecast
// loop, a generated ScaledObject, and an up-to-date status.
func (r *ForecastPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	key := workloadKey(req.Namespace, req.Name)

	var policy kedastralv1alpha1.ForecastPolicy
	if err := r.Get(ctx, req.NamespacedName, &policy); err != nil {
		if apierrors.IsNotFound(err) {
			r.Manager.Remove(key)
			r.Logger.Info("forecastpolicy deleted", "policy", req.NamespacedName.String(), "workload", key)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	var dataSource kedastralv1alpha1.DataSource
	dsKey := types.NamespacedName{Namespace: req.Namespace, Name: policy.Spec.DataSourceRef.Name}
	if err := r.Get(ctx, dsKey, &dataSource); err != nil {
		if apierrors.IsNotFound(err) {
			return r.fail(ctx, &policy, "DataSourceNotFound", fmt.Sprintf("DataSource %q not found", policy.Spec.DataSourceRef.Name))
		}
		return ctrl.Result{}, err
	}

	workloadConfig, err := toWorkloadConfig(&policy, &dataSource)
	if err != nil {
		return r.fail(ctx, &policy, "InvalidSpec", err.Error())
	}

	if err := r.Manager.Upsert(ctx, workloadConfig); err != nil {
		return r.fail(ctx, &policy, "ForecasterError", err.Error())
	}

	scaledObjectName, err := r.reconcileScaledObject(ctx, &policy, workloadConfig.Name)
	if err != nil {
		return r.fail(ctx, &policy, "ScaledObjectError", err.Error())
	}

	if err := r.updateReadyStatus(ctx, &policy, workloadConfig.Name, scaledObjectName); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: workloadConfig.Interval}, nil
}

// fail records a not-ready condition and requeues so the policy self-heals.
func (r *ForecastPolicyReconciler) fail(ctx context.Context, policy *kedastralv1alpha1.ForecastPolicy, reason, message string) (ctrl.Result, error) {
	r.Logger.Warn("forecastpolicy not ready", "policy", policy.Namespace+"/"+policy.Name, "reason", reason, "message", message)

	meta.SetStatusCondition(&policy.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: policy.Generation,
	})
	policy.Status.ObservedGeneration = policy.Generation

	if err := r.Status().Update(ctx, policy); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: failureRequeueInterval}, nil
}

// updateReadyStatus records a ready condition and the latest forecast outcome.
func (r *ForecastPolicyReconciler) updateReadyStatus(ctx context.Context, policy *kedastralv1alpha1.ForecastPolicy, workload, scaledObjectName string) error {
	policy.Status.ObservedGeneration = policy.Generation
	policy.Status.ScaledObjectName = scaledObjectName

	if snapshot, found, err := r.Store.GetLatest(ctx, workload); err == nil && found {
		generatedAt := metav1.NewTime(snapshot.GeneratedAt)
		policy.Status.LastForecastTime = &generatedAt
		if len(snapshot.DesiredReplicas) > 0 {
			policy.Status.CurrentReplicas = snapshot.DesiredReplicas[0]
			policy.Status.DesiredReplicas = maxInt(snapshot.DesiredReplicas)
		}
	}

	meta.SetStatusCondition(&policy.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            "Forecast loop running and ScaledObject reconciled",
		ObservedGeneration: policy.Generation,
	})

	return r.Status().Update(ctx, policy)
}

// SetupWithManager registers the reconciler, watching ForecastPolicies directly and
// DataSources via a mapping to their dependent policies.
func (r *ForecastPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kedastralv1alpha1.ForecastPolicy{}).
		Watches(&kedastralv1alpha1.DataSource{}, handler.EnqueueRequestsFromMapFunc(r.policiesForDataSource)).
		Complete(r)
}

// policiesForDataSource maps a DataSource event to reconcile requests for every
// ForecastPolicy in the same namespace that references it.
func (r *ForecastPolicyReconciler) policiesForDataSource(ctx context.Context, obj client.Object) []reconcile.Request {
	var policies kedastralv1alpha1.ForecastPolicyList
	if err := r.List(ctx, &policies, client.InNamespace(obj.GetNamespace())); err != nil {
		r.Logger.Error("failed to list forecastpolicies for datasource", "datasource", obj.GetName(), "error", err)
		return nil
	}

	var requests []reconcile.Request
	for i := range policies.Items {
		if policies.Items[i].Spec.DataSourceRef.Name != obj.GetName() {
			continue
		}
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: policies.Items[i].Namespace,
				Name:      policies.Items[i].Name,
			},
		})
	}
	return requests
}

func maxInt(values []int) int {
	highest := values[0]
	for _, v := range values[1:] {
		if v > highest {
			highest = v
		}
	}
	return highest
}
