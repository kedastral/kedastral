package main

import (
	"context"
	"fmt"
	"log/slog"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/HatiCode/kedastral/cmd/forecaster/config"
	"github.com/HatiCode/kedastral/cmd/forecaster/controller"
	kedastralv1alpha1 "github.com/HatiCode/kedastral/pkg/api/v1alpha1"
	"github.com/HatiCode/kedastral/pkg/storage"
)

// forecasterManager adapts the dynamic MultiForecaster to the controller's
// ForecasterManager interface, building a workload forecaster on each upsert.
type forecasterManager struct {
	multiForecaster *MultiForecaster
	store           storage.Store
	logger          *slog.Logger
}

func (m *forecasterManager) Upsert(_ context.Context, workloadConfig config.WorkloadConfig) error {
	forecaster, err := buildWorkloadForecaster(workloadConfig, m.store, m.logger)
	if err != nil {
		return err
	}
	m.multiForecaster.Upsert(forecaster)
	return nil
}

func (m *forecasterManager) Remove(name string) {
	m.multiForecaster.Remove(name)
}

// runOperator starts the controller-runtime manager that reconciles ForecastPolicy
// and DataSource resources. It blocks until the context is canceled.
func runOperator(ctx context.Context, cfg *config.Config, store storage.Store, multiForecaster *MultiForecaster, logger *slog.Logger) error {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(kedastralv1alpha1.AddToScheme(scheme))

	restConfig, err := ctrl.GetConfig()
	if err != nil {
		return fmt.Errorf("load kubernetes config: %w", err)
	}

	// The forecaster already serves /metrics, so the manager's metrics server is disabled.
	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme:  scheme,
		Metrics: metricsserver.Options{BindAddress: "0"},
	})
	if err != nil {
		return fmt.Errorf("create controller manager: %w", err)
	}

	reconciler := &controller.ForecastPolicyReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		Manager:       &forecasterManager{multiForecaster: multiForecaster, store: store, logger: logger},
		Store:         store,
		ScalerAddress: cfg.ScalerAddress,
		Logger:        logger,
	}
	if err := reconciler.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setup reconciler: %w", err)
	}

	logger.Info("controller manager started")
	return mgr.Start(ctx)
}
