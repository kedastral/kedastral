package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kedastralv1alpha1 "github.com/HatiCode/kedastral/pkg/api/v1alpha1"
)

type fakePolicyReader struct {
	policies []kedastralv1alpha1.ForecastPolicy
	err      error
}

func (f *fakePolicyReader) List(_ context.Context, namespace string) ([]kedastralv1alpha1.ForecastPolicy, error) {
	if f.err != nil {
		return nil, f.err
	}
	if namespace == "" {
		return f.policies, nil
	}
	var out []kedastralv1alpha1.ForecastPolicy
	for _, p := range f.policies {
		if p.Namespace == namespace {
			out = append(out, p)
		}
	}
	return out, nil
}

func (f *fakePolicyReader) Get(_ context.Context, namespace, name string) (*kedastralv1alpha1.ForecastPolicy, error) {
	if f.err != nil {
		return nil, f.err
	}
	for i := range f.policies {
		if f.policies[i].Namespace == namespace && f.policies[i].Name == name {
			return &f.policies[i], nil
		}
	}
	return nil, fmt.Errorf("forecastpolicies.kedastral.io %q not found", name)
}

func samplePolicy() kedastralv1alpha1.ForecastPolicy {
	forecastTime := metav1.NewTime(time.Now().Add(-30 * time.Second))
	return kedastralv1alpha1.ForecastPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "web-api", Namespace: "shop"},
		Spec: kedastralv1alpha1.ForecastPolicySpec{
			ScaleTargetRef: kedastralv1alpha1.ScaleTargetRef{Kind: "Deployment", Name: "web-api"},
			Metric:         "http_rps",
			DataSourceRef:  kedastralv1alpha1.DataSourceRef{Name: "prometheus"},
			Model:          kedastralv1alpha1.ModelSpec{Type: "sarima"},
			Capacity:       kedastralv1alpha1.CapacitySpec{TargetPerPod: 100, MinReplicas: 2, MaxReplicas: 20},
		},
		Status: kedastralv1alpha1.ForecastPolicyStatus{
			CurrentReplicas:  4,
			DesiredReplicas:  9,
			LastForecastTime: &forecastTime,
			ScaledObjectName: "web-api",
			Conditions: []metav1.Condition{{
				Type:    "Ready",
				Status:  metav1.ConditionTrue,
				Reason:  "Reconciled",
				Message: "Forecast loop running and ScaledObject reconciled",
			}},
		},
	}
}

func TestHandleListForecastPolicies_Success(t *testing.T) {
	reader := &fakePolicyReader{policies: []kedastralv1alpha1.ForecastPolicy{samplePolicy()}}
	handler := handleListForecastPolicies(reader, discardLogger())

	result, err := handler(context.Background(), callToolRequest(nil))
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}

	text := extractText(t, result)
	for _, want := range []string{"shop/web-api", "web-api", "sarima", "True", "current 4", "desired 9"} {
		if !containsStr(text, want) {
			t.Errorf("expected %q in output, got:\n%s", want, text)
		}
	}
}

func TestHandleListForecastPolicies_Empty(t *testing.T) {
	reader := &fakePolicyReader{}
	handler := handleListForecastPolicies(reader, discardLogger())

	result, err := handler(context.Background(), callToolRequest(nil))
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if !containsStr(extractText(t, result), "No ForecastPolicy") {
		t.Errorf("expected empty message, got %q", extractText(t, result))
	}
}

func TestHandleListForecastPolicies_Error(t *testing.T) {
	reader := &fakePolicyReader{err: fmt.Errorf("forbidden")}
	handler := handleListForecastPolicies(reader, discardLogger())

	result, err := handler(context.Background(), callToolRequest(nil))
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if !result.IsError {
		t.Fatal("expected tool error when reader fails")
	}
}

func TestHandleGetForecastPolicy_Success(t *testing.T) {
	reader := &fakePolicyReader{policies: []kedastralv1alpha1.ForecastPolicy{samplePolicy()}}
	handler := handleGetForecastPolicy(reader, discardLogger())

	result, err := handler(context.Background(), callToolRequest(map[string]any{"name": "web-api", "namespace": "shop"}))
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}

	text := extractText(t, result)
	for _, want := range []string{"ForecastPolicy shop/web-api", "Ready=True", "scaledObject:    web-api", "Reconciled"} {
		if !containsStr(text, want) {
			t.Errorf("expected %q in output, got:\n%s", want, text)
		}
	}
}

func TestHandleGetForecastPolicy_MissingName(t *testing.T) {
	reader := &fakePolicyReader{}
	handler := handleGetForecastPolicy(reader, discardLogger())

	result, err := handler(context.Background(), callToolRequest(nil))
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if !result.IsError {
		t.Fatal("expected tool error when name is missing")
	}
}

func TestHandleGetForecastPolicy_NotFound(t *testing.T) {
	reader := &fakePolicyReader{policies: []kedastralv1alpha1.ForecastPolicy{samplePolicy()}}
	handler := handleGetForecastPolicy(reader, discardLogger())

	result, err := handler(context.Background(), callToolRequest(map[string]any{"name": "missing", "namespace": "shop"}))
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if !result.IsError {
		t.Fatal("expected tool error when policy not found")
	}
}
