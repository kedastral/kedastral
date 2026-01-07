package models

import (
	"log/slog"
	"os"

	"github.com/HatiCode/kedastral/cmd/forecaster/config"
	"github.com/HatiCode/kedastral/pkg/models"
)

// New creates a forecasting model from the main config (single-workload mode).
func New(cfg *config.Config, logger *slog.Logger) models.Model {
	stepSec := int(cfg.Step.Seconds())
	horizonSec := int(cfg.Horizon.Seconds())

	switch cfg.Model {
	case "arima":
		logger.Info("initializing ARIMA model",
			"p", cfg.ARIMA_P,
			"d", cfg.ARIMA_D,
			"q", cfg.ARIMA_Q,
		)
		return models.NewARIMAModel(cfg.Metric, stepSec, horizonSec, cfg.ARIMA_P, cfg.ARIMA_D, cfg.ARIMA_Q)

	case "sarima":
		logger.Info("initializing SARIMA model",
			"p", cfg.SARIMA_P,
			"d", cfg.SARIMA_D,
			"q", cfg.SARIMA_Q,
			"P", cfg.SARIMA_SP,
			"D", cfg.SARIMA_SD,
			"Q", cfg.SARIMA_SQ,
			"s", cfg.SARIMA_S,
		)
		return models.NewSARIMAModel(cfg.Metric, stepSec, horizonSec,
			cfg.SARIMA_P, cfg.SARIMA_D, cfg.SARIMA_Q,
			cfg.SARIMA_SP, cfg.SARIMA_SD, cfg.SARIMA_SQ, cfg.SARIMA_S)

	case "baseline":
		logger.Info("initializing baseline model")
		return models.NewBaselineModel(cfg.Metric, stepSec, horizonSec)

	case "byom":
		logger.Info("initializing BYOM model", "url", cfg.BYOMURL)
		return models.NewBYOMModel(cfg.BYOMURL, cfg.Metric, stepSec, horizonSec)

	default:
		logger.Error("invalid model type", "model", cfg.Model)
		os.Exit(1)
	}

	return nil
}

// NewForWorkload creates a forecasting model from a workload config (multi-workload mode).
func NewForWorkload(wc config.WorkloadConfig, logger *slog.Logger) models.Model {
	stepSec := int(wc.Step.Seconds())
	horizonSec := int(wc.Horizon.Seconds())

	switch wc.Model {
	case "arima":
		logger.Info("initializing ARIMA model",
			"workload", wc.Name,
			"p", wc.ARIMA_P,
			"d", wc.ARIMA_D,
			"q", wc.ARIMA_Q,
		)
		return models.NewARIMAModel(wc.Metric, stepSec, horizonSec, wc.ARIMA_P, wc.ARIMA_D, wc.ARIMA_Q)

	case "sarima":
		logger.Info("initializing SARIMA model",
			"workload", wc.Name,
			"p", wc.SARIMA_P,
			"d", wc.SARIMA_D,
			"q", wc.SARIMA_Q,
			"P", wc.SARIMA_SP,
			"D", wc.SARIMA_SD,
			"Q", wc.SARIMA_SQ,
			"s", wc.SARIMA_S,
		)
		return models.NewSARIMAModel(wc.Metric, stepSec, horizonSec,
			wc.SARIMA_P, wc.SARIMA_D, wc.SARIMA_Q,
			wc.SARIMA_SP, wc.SARIMA_SD, wc.SARIMA_SQ, wc.SARIMA_S)

	case "baseline":
		logger.Info("initializing baseline model", "workload", wc.Name)
		return models.NewBaselineModel(wc.Metric, stepSec, horizonSec)

	case "byom":
		logger.Info("initializing BYOM model", "workload", wc.Name, "url", wc.BYOMURL)
		return models.NewBYOMModel(wc.BYOMURL, wc.Metric, stepSec, horizonSec)

	default:
		logger.Error("invalid model type", "model", wc.Model, "workload", wc.Name)
		os.Exit(1)
	}

	return nil
}
