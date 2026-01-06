# BYOM (Bring Your Own Model)

BYOM allows you to integrate any custom forecasting model with Kedastral by implementing a simple HTTP contract. This enables you to use advanced models like Prophet, TensorFlow, or custom algorithms without adding them to the core Kedastral codebase.

## Overview

Kedastral provides three built-in models:
- **baseline** - Simple moving average with seasonality detection (default)
- **arima** - ARIMA time-series model
- **byom** - Delegates predictions to an external HTTP service

The BYOM model type acts as an HTTP client that calls your custom model service, allowing you to use any forecasting model or framework in any language.

## Architecture

```
┌─────────────┐         ┌──────────────┐         ┌─────────────────┐
│  Forecaster │ ──────> │  BYOM Model  │ ──────> │  Your Service   │
│             │  train  │   (client)   │  HTTP   │   (Prophet,     │
│             │ predict │              │  POST   │   TensorFlow,   │
│             │ <────── │              │ <────── │   Custom, etc)  │
└─────────────┘         └──────────────┘         └─────────────────┘
```

When configured with `model: byom`, Kedastral will:
1. Collect historical metrics from Prometheus
2. Send them to your BYOM service via HTTP POST
3. Receive predictions from your service
4. Use those predictions for capacity planning

## HTTP Contract

### Request Format

```json
POST /predict
Content-Type: application/json

{
  "now": "2025-01-01T00:00:00Z",
  "horizonSeconds": 1800,
  "stepSeconds": 60,
  "features": [
    {
      "ts": "2025-01-01T00:00:00Z",
      "value": 100.0
    },
    {
      "ts": "2025-01-01T00:01:00Z",
      "value": 105.0
    }
  ]
}
```

**Fields:**
- `now` - Current timestamp (RFC3339 format)
- `horizonSeconds` - How far ahead to forecast (e.g., 1800 = 30 minutes)
- `stepSeconds` - Interval between predictions (e.g., 60 = 1 minute steps)
- `features` - Historical data points with timestamps and values

### Response Format

```json
HTTP/1.1 200 OK
Content-Type: application/json

{
  "metric": "my_forecast",
  "values": [420.0, 415.2, 430.9, ...]
}
```

**Fields:**
- `metric` - Name of the forecast (informational)
- `values` - Array of predictions (length must equal `horizonSeconds / stepSeconds`)

### Error Responses

Return HTTP 4xx/5xx with JSON error:

```json
{
  "error": "description of what went wrong"
}
```

## Configuration

### Single Workload Mode

```bash
./forecaster \
  --model=byom \
  --byom-url=http://my-model-service:8082/predict \
  --workload=my-api \
  --metric=http_rps \
  --prom-url=http://prometheus:9090 \
  --prom-query='rate(http_requests_total[5m])'
```

### Multi-Workload Mode

```yaml
workloads:
  - name: my-api
    metric: http_rps
    model: byom
    byomURL: http://my-model-service:8082/predict
    prometheusURL: http://prometheus:9090
    prometheusQuery: rate(http_requests_total{service="my-api"}[5m])
    horizon: 30m
    step: 1m
    interval: 30s
    window: 2h
    targetPerPod: 100
    minReplicas: 2
    maxReplicas: 50
```

## Prophet Example

Kedastral provides a reference implementation using Facebook Prophet at `examples/prophet-byom/`.

### Quick Start

```bash
# Build the Prophet service
cd examples/prophet-byom
docker build -t kedastral-prophet:latest .

# Deploy to Kubernetes
kubectl apply -f deployment.yaml

# Configure Kedastral forecaster
./forecaster \
  --model=byom \
  --byom-url=http://kedastral-prophet.kedastral.svc.cluster.local:8082/predict \
  --workload=my-api \
  --metric=http_rps \
  --prom-url=http://prometheus:9090 \
  --prom-query='rate(http_requests_total[5m])'
```

See [examples/prophet-byom/README.md](../examples/prophet-byom/README.md) for full details.

## Building Your Own BYOM Service

### 1. Choose Your Tech Stack

You can use any language or framework:
- **Python** - scikit-learn, TensorFlow, PyTorch, statsmodels
- **R** - forecast, fable, prophet
- **Go** - gonum, gota
- **JavaScript** - TensorFlow.js

### 2. Implement the HTTP Contract

Create a web service with a `POST /predict` endpoint that:
1. Accepts the request format above
2. Trains your model on the `features` data
3. Generates predictions for the requested horizon
4. Returns predictions in the response format

### 3. Handle Edge Cases

- Validate input parameters (horizonSeconds > 0, etc.)
- Ensure predictions array has correct length
- Clamp negative predictions to 0 (can't have negative load)
- Handle insufficient historical data gracefully
- Set reasonable timeouts (Kedastral uses 30s default)

### 4. Deploy Alongside Kedastral

Deploy your BYOM service to Kubernetes and configure Kedastral to call it via `--byom-url`.

## Best Practices

### Performance

- **Keep inference fast** - Kedastral calls your service every forecast interval (default 30s)
- **Consider caching** - Cache models or intermediate results if training is expensive
- **Use multiple workers** - Run multiple instances or worker processes for high availability

### Reliability

- **Implement health checks** - Provide a `/healthz` endpoint for Kubernetes probes
- **Handle errors gracefully** - Return informative error messages with 4xx/5xx codes
- **Add timeouts** - Set reasonable timeouts for training and inference
- **Monitor your service** - Track latency, errors, and prediction quality

### Model Quality

- **Validate predictions** - Ensure predictions make sense (no negatives, reasonable ranges)
- **Log predictions** - Log inputs and outputs for debugging and analysis
- **Consider quantiles** - Future Kedastral versions will support quantile forecasts for uncertainty
- **Test offline** - Validate your model on historical data before deploying

## Advanced Topics

### Custom Features

The `features` array contains the raw data from Prometheus. You can extend this by:
1. Adding custom feature engineering in your BYOM service
2. Using additional regressors (holidays, events, etc.)
3. Incorporating external data sources

### Quantile Forecasts

Future Kedastral versions will support quantile forecasts for uncertainty estimation. Your BYOM service could return:

```json
{
  "metric": "my_forecast",
  "values": [420.0, 415.2, ...],
  "quantiles": {
    "0.90": [450.0, 445.0, ...],
    "0.95": [480.0, 475.0, ...]
  }
}
```

### Multi-Model Ensembles

You could combine multiple models in your BYOM service:
- Average predictions from Prophet, ARIMA, and LSTM
- Use different models for different time horizons
- Implement model selection based on data characteristics

## Troubleshooting

### "byom: http request failed"

- Check that your BYOM service is running and accessible
- Verify the `--byom-url` is correct
- Check network connectivity and DNS resolution

### "byom: http 500: internal server error"

- Check your BYOM service logs for errors
- Ensure your service can handle the feature data format
- Verify sufficient resources (CPU, memory)

### "byom: expected N predictions, got M"

- Your service must return exactly `horizonSeconds / stepSeconds` predictions
- Check the response array length in your service

### Predictions are always zero

- Verify your model is training correctly
- Check for negative predictions being clamped to zero
- Ensure feature data contains valid values

## References

- [BYOM HTTP Contract (CLAUDE.md)](../.claude/CLAUDE.md#42-byom-http-optional)
- [Prophet Example](../examples/prophet-byom/)
- [Model Interface](../pkg/models/model.go)
