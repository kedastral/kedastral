package controller

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/HatiCode/kedastral/cmd/forecaster/config"
	kedastralv1alpha1 "github.com/HatiCode/kedastral/pkg/api/v1alpha1"
	"github.com/HatiCode/kedastral/pkg/storage"
)

// fakeManager records Upsert/Remove calls for assertions.
type fakeManager struct {
	mu       sync.Mutex
	upserted []config.WorkloadConfig
	removed  []string
}

func (m *fakeManager) Upsert(_ context.Context, wc config.WorkloadConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.upserted = append(m.upserted, wc)
	return nil
}

func (m *fakeManager) Remove(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removed = append(m.removed, name)
}

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := kedastralv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	scheme.AddKnownTypeWithName(scaledObjectGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(scaledObjectGVK.GroupVersion().WithKind("ScaledObjectList"), &unstructured.UnstructuredList{})
	return scheme
}

func newReconciler(t *testing.T, manager ForecasterManager, store storage.Store, objs ...runtime.Object) *ForecastPolicyReconciler {
	t.Helper()
	scheme := testScheme(t)
	builder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&kedastralv1alpha1.ForecastPolicy{})
	for _, o := range objs {
		builder = builder.WithRuntimeObjects(o)
	}
	return &ForecastPolicyReconciler{
		Client:        builder.Build(),
		Scheme:        scheme,
		Manager:       manager,
		Store:         store,
		ScalerAddress: "kedastral-scaler.kedastral:50051",
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func reconcileRequest(namespace, name string) ctrl.Request {
	return ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: name}}
}

func TestReconcile_Success(t *testing.T) {
	manager := &fakeManager{}
	store := storage.NewMemoryStore()
	r := newReconciler(t, manager, store, basePolicy(), promDataSource())

	if _, err := r.Reconcile(context.Background(), reconcileRequest("shop", "web")); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if len(manager.upserted) != 1 {
		t.Fatalf("Upsert called %d times, want 1", len(manager.upserted))
	}
	if manager.upserted[0].Name != "shop-web" {
		t.Errorf("upserted workload = %q, want shop-web", manager.upserted[0].Name)
	}

	// ScaledObject created, owned by the policy, with the right trigger metadata.
	so := &unstructured.Unstructured{}
	so.SetGroupVersionKind(scaledObjectGVK)
	if err := r.Get(context.Background(), types.NamespacedName{Namespace: "shop", Name: "web"}, so); err != nil {
		t.Fatalf("ScaledObject not created: %v", err)
	}

	owners := so.GetOwnerReferences()
	if len(owners) != 1 || owners[0].Kind != "ForecastPolicy" || owners[0].Name != "web" {
		t.Errorf("owner references = %v, want controller ref to ForecastPolicy/web", owners)
	}

	triggers, _, _ := unstructured.NestedSlice(so.Object, "spec", "triggers")
	if len(triggers) != 1 {
		t.Fatalf("triggers = %v, want 1", triggers)
	}
	trigger := triggers[0].(map[string]any)
	metadata := trigger["metadata"].(map[string]any)
	if metadata["workload"] != "shop-web" {
		t.Errorf("trigger workload = %v, want shop-web", metadata["workload"])
	}
	if metadata["scalerAddress"] != "kedastral-scaler.kedastral:50051" {
		t.Errorf("trigger scalerAddress = %v", metadata["scalerAddress"])
	}

	// Status reflects readiness.
	var updated kedastralv1alpha1.ForecastPolicy
	if err := r.Get(context.Background(), types.NamespacedName{Namespace: "shop", Name: "web"}, &updated); err != nil {
		t.Fatalf("get policy: %v", err)
	}
	if updated.Status.ScaledObjectName != "web" {
		t.Errorf("status ScaledObjectName = %q, want web", updated.Status.ScaledObjectName)
	}
	if len(updated.Status.Conditions) == 0 || updated.Status.Conditions[0].Type != "Ready" {
		t.Errorf("expected Ready condition, got %v", updated.Status.Conditions)
	}
}

func TestReconcile_DataSourceNotFound(t *testing.T) {
	manager := &fakeManager{}
	store := storage.NewMemoryStore()
	r := newReconciler(t, manager, store, basePolicy()) // no DataSource

	res, err := r.Reconcile(context.Background(), reconcileRequest("shop", "web"))
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if res.RequeueAfter == 0 {
		t.Error("expected requeue when datasource missing")
	}
	if len(manager.upserted) != 0 {
		t.Errorf("Upsert should not be called when datasource missing, got %d", len(manager.upserted))
	}

	var updated kedastralv1alpha1.ForecastPolicy
	if err := r.Get(context.Background(), types.NamespacedName{Namespace: "shop", Name: "web"}, &updated); err != nil {
		t.Fatalf("get policy: %v", err)
	}
	if len(updated.Status.Conditions) == 0 || updated.Status.Conditions[0].Reason != "DataSourceNotFound" {
		t.Errorf("expected DataSourceNotFound condition, got %v", updated.Status.Conditions)
	}
}

func TestReconcile_DeletedPolicyRemovesForecaster(t *testing.T) {
	manager := &fakeManager{}
	store := storage.NewMemoryStore()
	r := newReconciler(t, manager, store) // no policy in store -> treated as deleted

	if _, err := r.Reconcile(context.Background(), reconcileRequest("shop", "web")); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if len(manager.removed) != 1 || manager.removed[0] != "shop-web" {
		t.Errorf("Remove calls = %v, want [shop-web]", manager.removed)
	}
}
