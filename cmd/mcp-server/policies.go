package main

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	kedastralv1alpha1 "github.com/HatiCode/kedastral/pkg/api/v1alpha1"
)

// PolicyReader reads ForecastPolicy resources from the cluster. It is satisfied by a
// controller-runtime client in production and by a fake in tests.
type PolicyReader interface {
	List(ctx context.Context, namespace string) ([]kedastralv1alpha1.ForecastPolicy, error)
	Get(ctx context.Context, namespace, name string) (*kedastralv1alpha1.ForecastPolicy, error)
}

// k8sPolicyReader reads ForecastPolicies via a controller-runtime client.
type k8sPolicyReader struct {
	client crclient.Client
}

// newPolicyReader builds a PolicyReader from the ambient Kubernetes config (in-cluster
// or kubeconfig). It returns an error when no cluster access is available, in which case
// the operator-aware tools are simply not registered.
func newPolicyReader() (PolicyReader, error) {
	restConfig, err := ctrl.GetConfig()
	if err != nil {
		return nil, err
	}

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(kedastralv1alpha1.AddToScheme(scheme))

	c, err := crclient.New(restConfig, crclient.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}
	return &k8sPolicyReader{client: c}, nil
}

func (r *k8sPolicyReader) List(ctx context.Context, namespace string) ([]kedastralv1alpha1.ForecastPolicy, error) {
	var list kedastralv1alpha1.ForecastPolicyList
	var opts []crclient.ListOption
	if namespace != "" {
		opts = append(opts, crclient.InNamespace(namespace))
	}
	if err := r.client.List(ctx, &list, opts...); err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (r *k8sPolicyReader) Get(ctx context.Context, namespace, name string) (*kedastralv1alpha1.ForecastPolicy, error) {
	var policy kedastralv1alpha1.ForecastPolicy
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &policy); err != nil {
		return nil, err
	}
	return &policy, nil
}

// registerPolicyTools adds the operator-aware tools when a reader is available.
func registerPolicyTools(s *server.MCPServer, reader PolicyReader, log *slog.Logger) {
	s.AddTool(
		mcp.NewTool("list_forecast_policies",
			mcp.WithDescription("List Kedastral ForecastPolicy resources and their status (target workload, model, readiness, current/desired replicas). Requires the operator."),
			mcp.WithString("namespace",
				mcp.Description("Namespace to list (omit for all namespaces)"),
			),
		),
		handleListForecastPolicies(reader, log),
	)

	s.AddTool(
		mcp.NewTool("get_forecast_policy",
			mcp.WithDescription("Get a ForecastPolicy's full configuration and status, including readiness conditions and the generated ScaledObject. Useful for explaining why a policy is or isn't forecasting."),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Name of the ForecastPolicy"),
			),
			mcp.WithString("namespace",
				mcp.Description("Namespace of the ForecastPolicy (default: default)"),
			),
		),
		handleGetForecastPolicy(reader, log),
	)
}

func handleListForecastPolicies(reader PolicyReader, log *slog.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		namespace := req.GetString("namespace", "")

		policies, err := reader.List(ctx, namespace)
		if err != nil {
			log.Error("list_forecast_policies failed", "error", err)
			return mcp.NewToolResultError(fmt.Sprintf("failed to list forecast policies: %v", err)), nil
		}

		if len(policies) == 0 {
			return mcp.NewToolResultText("No ForecastPolicy resources found."), nil
		}

		sort.Slice(policies, func(i, j int) bool {
			if policies[i].Namespace != policies[j].Namespace {
				return policies[i].Namespace < policies[j].Namespace
			}
			return policies[i].Name < policies[j].Name
		})

		var sb strings.Builder
		fmt.Fprintf(&sb, "ForecastPolicies (%d):\n", len(policies))
		for i := range policies {
			policy := &policies[i]
			ready, reason := readyState(policy)
			fmt.Fprintf(&sb, "\n- %s/%s\n", policy.Namespace, policy.Name)
			fmt.Fprintf(&sb, "    target:   %s\n", policy.Spec.ScaleTargetRef.Name)
			fmt.Fprintf(&sb, "    model:    %s\n", modelType(policy))
			fmt.Fprintf(&sb, "    ready:    %s (%s)\n", ready, reason)
			fmt.Fprintf(&sb, "    replicas: current %d, desired %d\n", policy.Status.CurrentReplicas, policy.Status.DesiredReplicas)
			fmt.Fprintf(&sb, "    forecast: %s\n", lastForecast(policy))
		}

		return mcp.NewToolResultText(sb.String()), nil
	}
}

func handleGetForecastPolicy(reader PolicyReader, log *slog.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := req.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError("name parameter is required"), nil
		}
		namespace := req.GetString("namespace", "default")

		policy, err := reader.Get(ctx, namespace, name)
		if err != nil {
			log.Error("get_forecast_policy failed", "namespace", namespace, "name", name, "error", err)
			return mcp.NewToolResultError(fmt.Sprintf("failed to get forecast policy %s/%s: %v", namespace, name, err)), nil
		}

		ready, reason := readyState(policy)

		var sb strings.Builder
		fmt.Fprintf(&sb, "ForecastPolicy %s/%s\n", policy.Namespace, policy.Name)
		fmt.Fprintf(&sb, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
		fmt.Fprintf(&sb, "Spec:\n")
		fmt.Fprintf(&sb, "  scaleTarget:  %s/%s\n", policy.Spec.ScaleTargetRef.Kind, policy.Spec.ScaleTargetRef.Name)
		fmt.Fprintf(&sb, "  metric:       %s\n", policy.Spec.Metric)
		fmt.Fprintf(&sb, "  dataSource:   %s\n", policy.Spec.DataSourceRef.Name)
		fmt.Fprintf(&sb, "  model:        %s\n", modelType(policy))
		fmt.Fprintf(&sb, "  replicas:     min %d, max %d\n", policy.Spec.Capacity.MinReplicas, policy.Spec.Capacity.MaxReplicas)
		fmt.Fprintf(&sb, "  targetPerPod: %.2f\n\n", policy.Spec.Capacity.TargetPerPod)

		fmt.Fprintf(&sb, "Status:\n")
		fmt.Fprintf(&sb, "  ready:           %s (%s)\n", ready, reason)
		fmt.Fprintf(&sb, "  currentReplicas: %d\n", policy.Status.CurrentReplicas)
		fmt.Fprintf(&sb, "  desiredReplicas: %d\n", policy.Status.DesiredReplicas)
		fmt.Fprintf(&sb, "  lastForecast:    %s\n", lastForecast(policy))
		if policy.Status.ScaledObjectName != "" {
			fmt.Fprintf(&sb, "  scaledObject:    %s\n", policy.Status.ScaledObjectName)
		}

		if len(policy.Status.Conditions) > 0 {
			fmt.Fprintf(&sb, "\nConditions:\n")
			for _, cond := range policy.Status.Conditions {
				fmt.Fprintf(&sb, "  - %s=%s: %s — %s\n", cond.Type, cond.Status, cond.Reason, cond.Message)
			}
		}

		return mcp.NewToolResultText(sb.String()), nil
	}
}

func readyState(policy *kedastralv1alpha1.ForecastPolicy) (status, reason string) {
	cond := meta.FindStatusCondition(policy.Status.Conditions, "Ready")
	if cond == nil {
		return "Unknown", "no status reported yet"
	}
	return string(cond.Status), cond.Reason
}

func modelType(policy *kedastralv1alpha1.ForecastPolicy) string {
	if policy.Spec.Model.Type == "" {
		return "baseline"
	}
	return policy.Spec.Model.Type
}

func lastForecast(policy *kedastralv1alpha1.ForecastPolicy) string {
	if policy.Status.LastForecastTime == nil {
		return "never"
	}
	age := time.Since(policy.Status.LastForecastTime.Time).Round(time.Second)
	return fmt.Sprintf("%s (%s ago)", policy.Status.LastForecastTime.Format(time.RFC3339), age)
}
