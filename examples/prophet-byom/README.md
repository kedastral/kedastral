# Prophet BYOM Example

This is a reference implementation of a BYOM (Bring Your Own Model) service for Kedastral using Facebook Prophet.

## Overview

This service implements the BYOM HTTP contract defined in the Kedastral documentation. It allows Kedastral to use Prophet for time-series forecasting without requiring Prophet to be part of the core Kedastral codebase.

## Endpoints

- `GET /healthz` - Health check
- `POST /predict` - Generate forecasts using Prophet

## Running Locally

```bash
# Install dependencies
pip install -r requirements.txt

# Run the service
python app.py

# Or use gunicorn for production
gunicorn -w 4 -b 0.0.0.0:8082 app:app
```

## Running with Docker

```bash
# Build the image
docker build -t kedastral-prophet:latest .

# Run the container
docker run -p 8082:8082 kedastral-prophet:latest
```

## Using with Kedastral

Configure Kedastral to use this BYOM service:

```bash
# Single workload mode
./forecaster \
  --model=byom \
  --byom-url=http://prophet-service:8082/predict \
  --workload=my-api \
  --metric=http_rps \
  --prom-url=http://prometheus:9090 \
  --prom-query='rate(http_requests_total[5m])'
```

Or in multi-workload mode YAML:

```yaml
workloads:
  - name: my-api
    metric: http_rps
    model: byom
    byomURL: http://prophet-service:8082/predict
    prometheusURL: http://prometheus:9090
    prometheusQuery: rate(http_requests_total[5m])
    horizon: 30m
    step: 1m
    # ... other config
```

## BYOM Contract

The service implements the BYOM HTTP contract:

### Request Format
```json
{
  "now": "2025-01-01T00:00:00Z",
  "horizonSeconds": 1800,
  "stepSeconds": 60,
  "features": [
    {"ts": "2025-01-01T00:00:00Z", "value": 100.0},
    {"ts": "2025-01-01T00:01:00Z", "value": 105.0}
  ]
}
```

### Response Format
```json
{
  "metric": "prophet_forecast",
  "values": [420.0, 415.2, 430.9, ...]
}
```

## Customization

This is a minimal reference implementation. For production use, consider:

- Adding authentication/authorization
- Implementing request caching
- Tuning Prophet hyperparameters for your use case
- Adding custom regressors or holidays
- Implementing quantile forecasts
- Adding metrics and monitoring

## References

- [Prophet Documentation](https://facebook.github.io/prophet/)
- [Kedastral Documentation](../../docs/)
