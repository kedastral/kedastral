// Command backtest replays a historical metric series (CSV of timestamp,value) through
// a Kedastral forecasting model and the capacity planner, reporting forecast accuracy
// and capacity outcomes. It is an offline evaluation tool for comparing models and
// tuning policy before deploying.
//
// Example:
//
//	backtest -input history.csv -model sarima -sarima-s=24 \
//	  -step=1m -horizon=30m -window=6h -target-per-pod=50
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/HatiCode/kedastral/pkg/backtest"
	"github.com/HatiCode/kedastral/pkg/capacity"
	"github.com/HatiCode/kedastral/pkg/durationx"
	"github.com/HatiCode/kedastral/pkg/models"
)

func main() {
	input := flag.String("input", "", "Path to CSV file (timestamp,value); reads stdin if empty")
	model := flag.String("model", "baseline", "Model: baseline, arima, sarima, or byom")
	metric := flag.String("metric", "value", "Metric name")
	output := flag.String("output", "text", "Output format: text or json")

	horizon := durationx.Duration("horizon", 30*time.Minute, "Forecast horizon")
	step := durationx.Duration("step", time.Minute, "Series step / forecast resolution")
	window := durationx.Duration("window", 6*time.Hour, "Training window")
	stride := durationx.Duration("stride", 0, "Evaluation stride (defaults to step)")

	arimaP := flag.Int("arima-p", 0, "ARIMA AR order (0=auto)")
	arimaD := flag.Int("arima-d", 0, "ARIMA differencing order (0=auto)")
	arimaQ := flag.Int("arima-q", 0, "ARIMA MA order (0=auto)")
	sarimaP := flag.Int("sarima-p", 0, "SARIMA AR order")
	sarimaD := flag.Int("sarima-d", 0, "SARIMA differencing order")
	sarimaQ := flag.Int("sarima-q", 0, "SARIMA MA order")
	sarimaSP := flag.Int("sarima-sp", 1, "SARIMA seasonal AR order")
	sarimaSD := flag.Int("sarima-sd", 1, "SARIMA seasonal differencing order")
	sarimaSQ := flag.Int("sarima-sq", 1, "SARIMA seasonal MA order")
	sarimaS := flag.Int("sarima-s", 24, "SARIMA seasonal period (steps)")
	byomURL := flag.String("byom-url", "", "BYOM service URL (required when model=byom)")

	targetPerPod := flag.Float64("target-per-pod", 100.0, "Target metric value per pod")
	headroom := flag.Float64("headroom", 1.2, "Headroom multiplier")
	quantileLevel := flag.String("quantile-level", "0", "Quantile level (p90, 0.95, or 0 to disable)")
	minReplicas := flag.Int("min", 1, "Minimum replicas")
	maxReplicas := flag.Int("max", 100, "Maximum replicas")
	upMaxFactor := flag.Float64("up-max-factor", 2.0, "Max scale-up factor per step")
	downMaxPercent := flag.Int("down-max-percent", 50, "Max scale-down percent per step")

	flag.Parse()

	stepSec := int(step.Seconds())
	horizonSec := int(horizon.Seconds())

	if *model == "byom" && *byomURL == "" {
		fail("--byom-url is required when --model=byom")
	}

	quantile, err := capacity.ParseQuantileLevel(*quantileLevel)
	if err != nil {
		fail(fmt.Sprintf("invalid quantile level: %v", err))
	}

	newModel, err := modelFactory(*model, *metric, stepSec, horizonSec, modelParams{
		arimaP: *arimaP, arimaD: *arimaD, arimaQ: *arimaQ,
		sarimaP: *sarimaP, sarimaD: *sarimaD, sarimaQ: *sarimaQ,
		sarimaSP: *sarimaSP, sarimaSD: *sarimaSD, sarimaSQ: *sarimaSQ, sarimaS: *sarimaS,
		byomURL: *byomURL,
	})
	if err != nil {
		fail(err.Error())
	}

	series, err := loadSeries(*input)
	if err != nil {
		fail(err.Error())
	}

	cfg := backtest.Config{
		Window:   *window,
		Horizon:  *horizon,
		Step:     *step,
		Stride:   *stride,
		NewModel: newModel,
		Policy: capacity.Policy{
			TargetPerPod:          *targetPerPod,
			Headroom:              *headroom,
			QuantileLevel:         quantile,
			MinReplicas:           *minReplicas,
			MaxReplicas:           *maxReplicas,
			UpMaxFactorPerStep:    *upMaxFactor,
			DownMaxPercentPerStep: *downMaxPercent,
		},
	}

	report, err := backtest.Run(context.Background(), series, cfg)
	if err != nil {
		fail(err.Error())
	}

	if *output == "json" {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fail(err.Error())
		}
		return
	}

	printText(report)
}

type modelParams struct {
	arimaP, arimaD, arimaQ                int
	sarimaP, sarimaD, sarimaQ             int
	sarimaSP, sarimaSD, sarimaSQ, sarimaS int
	byomURL                               string
}

func modelFactory(name, metric string, stepSec, horizonSec int, p modelParams) (func() models.Model, error) {
	switch name {
	case "baseline":
		return func() models.Model { return models.NewBaselineModel(metric, stepSec, horizonSec) }, nil
	case "arima":
		return func() models.Model {
			return models.NewARIMAModel(metric, stepSec, horizonSec, p.arimaP, p.arimaD, p.arimaQ)
		}, nil
	case "sarima":
		return func() models.Model {
			return models.NewSARIMAModel(metric, stepSec, horizonSec,
				p.sarimaP, p.sarimaD, p.sarimaQ, p.sarimaSP, p.sarimaSD, p.sarimaSQ, p.sarimaS)
		}, nil
	case "byom":
		return func() models.Model { return models.NewBYOMModel(p.byomURL, metric, stepSec, horizonSec) }, nil
	default:
		return nil, fmt.Errorf("invalid model %q (want baseline, arima, sarima, or byom)", name)
	}
}

func loadSeries(path string) (backtest.Series, error) {
	if path == "" {
		return backtest.LoadCSV(os.Stdin)
	}
	file, err := os.Open(path)
	if err != nil {
		return backtest.Series{}, fmt.Errorf("open input: %w", err)
	}
	defer file.Close()
	return backtest.LoadCSV(file)
}

func printText(r backtest.Report) {
	fmt.Printf("Backtest report\n")
	fmt.Printf("  model:                  %s\n", r.Model)
	fmt.Printf("  windows evaluated:      %d (skipped %d)\n", r.Windows, r.Skipped)
	fmt.Printf("  forecast points:        %d\n", r.Points)
	fmt.Printf("  forecast accuracy:\n")
	fmt.Printf("    MAE:                  %.4f\n", r.MAE)
	fmt.Printf("    RMSE:                 %.4f\n", r.RMSE)
	fmt.Printf("    MAPE:                 %.2f%%\n", r.MAPE)
	fmt.Printf("  capacity outcome:\n")
	fmt.Printf("    under-provisioned:    %.2f%% of steps\n", r.UnderProvisionedRate*100)
	fmt.Printf("    mean over-provision:  %.2f%%\n", r.MeanOverProvision*100)
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, "Error: "+message)
	os.Exit(1)
}
