# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.5] - 2026-01-06

### Added

- **Bring Your Own Model (BYOM)**: Support for external custom forecasting models via HTTP API
  - New `pkg/models/byom.go` implementing the Model interface with HTTP client for external predictions
  - BYOM HTTP contract: POST `/predict` with features and horizon, returns forecast values and optional quantiles
  - Model selection via `--model=byom` flag with `--byom-url` for external model endpoint
  - Comprehensive test suite with mock HTTP server covering success, errors, timeouts, and malformed responses
  - **Prophet Example**: Complete Python-based forecasting service using Facebook Prophet
    - Flask API implementing BYOM contract (`examples/prophet-byom/app.py`)
    - Automatic trend detection, seasonality (daily, weekly, yearly), and holiday effects
    - Quantile prediction support (p50, p75, p90, p95) using prediction intervals
    - Docker image with requirements: pandas, prophet, flask, gunicorn
    - Kubernetes deployment example with health checks and resource limits
    - Comprehensive README with local testing and deployment instructions
  - BYOM workload configuration example in `deploy/examples/workloads-byom.yaml`
  - Detailed documentation in `docs/byom.md` covering:
    - HTTP contract specification with request/response schemas
    - Authentication and security considerations (mTLS, API keys)
    - Error handling and retry logic (3 retries with exponential backoff)
    - Performance guidelines (1s timeout for training, 500ms for prediction)
    - Feature engineering guide for external models
    - Example implementations in Python (Prophet, ARIMA, LSTM)
    - Troubleshooting guide and best practices

### Changed

- **Documentation Restructure**: Reorganized documentation for better clarity and navigation
  - Condensed main README.md focusing on quick start and key features
  - Created dedicated `cmd/forecaster/README.md` (510 lines) with complete forecaster documentation
  - Created dedicated `cmd/scaler/README.md` (556 lines) with complete scaler documentation
  - New `docs/ARCHITECTURE.md` (356 lines) with system design and component interactions
  - New `docs/CONFIGURATION.md` (471 lines) with comprehensive configuration reference
  - New `docs/DEPLOYMENT.md` (701 lines) with production deployment guides
  - New `docs/OBSERVABILITY.md` (404 lines) with monitoring and alerting setup
  - New `docs/QUICKSTART.md` (238 lines) with step-by-step getting started guide
  - Removed outdated `docs/cli-design.md` (replaced by structured documentation)
  - Total documentation expansion: 3,405 lines added, 755 lines removed

### Dependencies

- Bumped Flask from 3.1.0 to 3.1.1 in `examples/prophet-byom` (security update)

### Documentation

- Added comprehensive BYOM guide with contract specs, security, and examples
- Restructured all documentation into focused, topic-specific files
- Added detailed README for Prophet BYOM example
- Enhanced deployment examples with BYOM workload configuration

## [0.1.4] - 2025-12-21

### Added

- **Multi-Workload Forecasting**: Single forecaster instance can now manage multiple workloads concurrently
  - New YAML-based configuration file for defining multiple workloads (`--config-file` flag)
  - Concurrent workload processing with per-workload isolation and panic recovery
  - Per-operation timeouts: collect (10s), train (5s), predict (2s), store (2s)
  - `WorkloadForecaster` and `MultiForecaster` types for clean separation of concerns
  - Example workloads configuration files in `deploy/examples/`
- **mTLS Security**: Mutual TLS authentication between scaler and forecaster
  - New `pkg/tls` package providing client and server TLS configuration
  - TLS 1.3 with strong cipher suites (TLS_AES_256_GCM_SHA384, TLS_AES_128_GCM_SHA256)
  - Client certificate verification (RequireAndVerifyClientCert)
  - Forecaster HTTP server supports mTLS (`--tls-enabled`, `--tls-cert-file`, `--tls-key-file`, `--tls-ca-file`)
  - Scaler HTTP client supports mTLS with same configuration flags
  - Integration with httpx package for unified HTTP client creation
- **Enhanced Configuration Validation**: Security-focused input validation
  - Workload name regex validation (DNS-compatible, 1-253 chars)
  - Context-based timeouts for all storage operations (2s default)
  - ConfigMap input validation in router with injection attack prevention
  - Comprehensive validation in `LoadWorkloads()` with detailed error messages
- **Helm Chart Enhancements**: Support for multi-workload and mTLS deployments
  - ConfigMap template for workloads configuration (`forecaster-configmap.yaml`)
  - cert-manager Certificate resources for automatic certificate provisioning
  - CA Issuer template for self-signed development certificates
  - TLS volume mounts in both forecaster and scaler deployments
  - Example values file for multi-workload mTLS setup (`values-multiworkload-mtls.yaml`)
  - Backward-compatible with single-workload mode
- **Quantile Forecasting**: Uncertainty-aware predictions with automatic quantile generation
  - Baseline model computes quantiles (p50, p75, p90, p95) from seasonal variance
  - ARIMA model computes quantiles from residual standard deviation with horizon-adjusted uncertainty
  - Quantiles automatically included in forecast snapshots (backward compatible)
  - Models generate 4 quantile levels using standard normal z-scores
- **Quantile-Based Capacity Planning**: Use forecast quantiles instead of fixed headroom multiplier
  - New `--quantile-level` flag for enabling quantile-based scaling (p90, p95, or 0.90, 0.95)
  - Intelligent fallback: uses quantile when available, headroom otherwise (opt-in design)
  - Per-workload configuration via YAML `quantileLevel` field or command-line flag
  - Policy struct extended with `QuantileLevel` field
- **p-Notation Support**: Industry-standard percentile notation for better UX
  - Automatic parsing of both p-notation (p90, p95) and decimal (0.90, 0.95) formats
  - User-friendly display in logs showing p-notation (e.g., "quantile-based capacity planning enabled, quantile=p90")
  - Helper functions: `capacity.ParseQuantileLevel()` and `capacity.FormatQuantileLevel()`
  - Validation of input ranges (percentile 0-100, quantile 0-1)

### Changed

- **Forecaster Architecture**: Refactored from single-workload to multi-workload design
  - `Forecaster` type renamed to `WorkloadForecaster` for clarity
  - Added `MultiForecaster` to coordinate multiple workload forecasters
  - Forecast loop now runs per-workload with independent goroutines
  - Interval parameter moved from `Run()` method to struct configuration
- **Storage Interface**: All methods now accept `context.Context` as first parameter
  - `Put(ctx context.Context, snapshot Snapshot) error`
  - `GetLatest(ctx context.Context, workload string) (Snapshot, bool, error)`
  - Updated memory and Redis implementations
  - All storage tests updated for context propagation
- **Scaler Constructor**: Now returns `(*Scaler, error)` to handle TLS initialization errors
  - TLS configuration passed as `tls.Config` parameter
  - HTTP client creation delegated to `httpx.NewClient()`
- **httpx Package**: Extended with TLS support
  - `SetTLSConfig(*tls.Config)` method for server TLS configuration
  - `StartTLS(certFile, keyFile string)` method for HTTPS server
  - `NewClient(tls.Config, timeout)` accepts custom TLS config (returns error)
  - Package remains independent of custom tls package (uses crypto/tls)
- **Forecast Model Interface**: Extended to support optional quantile predictions
  - `Forecast` struct now includes `Quantiles map[float64][]float64` field
  - Backward compatible: quantiles are optional, models work without them
  - Point forecast (Values) remains primary prediction, quantiles provide uncertainty bounds
- **Capacity Planner**: Signature updated to accept quantiles parameter
  - `ToReplicas()` now accepts `quantiles map[float64][]float64` as optional parameter
  - When `Policy.QuantileLevel > 0` and quantiles available, uses specified quantile directly
  - Otherwise falls back to point forecast × headroom (existing behavior)
  - All existing tests updated to pass `nil` for quantiles (backward compatibility)
- **Storage Snapshots**: Now include quantiles for transparency
  - `Snapshot` struct extended with `Quantiles` field (JSON tag: `omitempty`)
  - Forecaster automatically stores quantiles when available
  - Scaler can access quantiles from Redis/memory for analysis
- **Config Structs**: Added `QuantileLevel` field to Config and WorkloadConfig
  - Accepts both p-notation strings ("p90") and decimal strings ("0.90")
  - Default value "0" disables quantile-based planning (uses headroom)
  - Environment variable support: `QUANTILE_LEVEL`

### Security

- Implemented mutual TLS (mTLS) authentication between all components
- Added comprehensive input validation with DNS-compatible regex for workload names
- Per-operation timeout enforcement to prevent resource exhaustion
- Context-based cancellation support throughout storage layer
- TLS certificate validation with custom CA support
- Secure defaults: TLS 1.3 minimum, strong cipher suites only

### Fixed

- All forecaster tests updated for new `WorkloadForecaster` API
- All scaler tests updated for new constructor signature with TLS parameter
- Router tests updated for context-based storage operations
- Fixed workload field references (`workload` → `name`) in test structs
- Removed obsolete `GetStore()` and `GetWorkload()` test functions

### Documentation

- Created comprehensive workloads YAML examples (`workloads.yaml`, `workloads-documented.yaml`)
- Added Helm values example for multi-workload mTLS deployment
- Updated package documentation for new architecture
- Added detailed comments explaining TLS configuration options
- **Quantile Forecasting Guide**: New comprehensive documentation in `docs/planner/quantile-forecasting.md`
  - Detailed explanation of quantile vs headroom approaches
  - Configuration examples with both p-notation and decimal formats
  - Migration guide for existing deployments
  - Comparison scenarios showing adaptive vs fixed buffers
  - Advantages, limitations, and future enhancements

### Performance

- Concurrent workload processing improves throughput for multi-workload deployments
- Per-workload panic recovery prevents cascading failures
- Independent goroutines eliminate cross-workload blocking

## [0.1.3] - 2025-12-21

### Added

- **Production-Ready Helm Chart**: Complete Helm chart with deployment best practices
  - Comprehensive `values.yaml` with all configuration options
  - Support for both forecaster and scaler deployments
  - Resource limits and requests with recommended defaults
  - Health checks (liveness and readiness probes)
  - Configurable number of replicas and autoscaling support
  - Service configuration for both HTTP (forecaster) and gRPC (scaler)
  - RBAC configuration (ServiceAccount, Role, RoleBinding)
  - ConfigMap integration for externalized configuration
- Kubernetes manifest validation and linting
- Deployment examples with production configurations
- Release badge in README

### Changed

- Improved baseline model with better trend detection
- Enhanced lead-time logic in scaler for more accurate proactive scaling
- Updated organization references throughout documentation
- Refined model training for better consistency

### Fixed

- Helm chart linting errors
- Kubernetes manifest validation issues
- Baseline model training consistency issues

### Documentation

- Added Helm chart installation and configuration guide
- Updated README with release information
- Enhanced deployment examples with production best practices

## [0.1.2] - 2025-12-17

### Added

- **ARIMA Forecasting Model**: Added ARIMA (AutoRegressive Integrated Moving Average) as an alternative forecasting model for workloads with trends and seasonality
  - Pure Go implementation of ARIMA(p,d,q) algorithm with Yule-Walker equations and Levinson-Durbin solver
  - New `pkg/models/arima.go` implementing the Model interface with training and prediction capabilities
  - Comprehensive test suite with 16 tests covering constant, linear, seasonal, and complex time series patterns
  - Benchmark tests showing ~15μs training time per 1K points and <1μs prediction for 30 steps
  - Configurable ARIMA orders via `--arima-p`, `--arima-d`, `--arima-q` flags with auto-detection (defaults to 1,1,1)
  - Thread-safe concurrent predictions using sync.RWMutex
  - Numerical stability handling for edge cases (constant series, zero variance)
  - Non-negativity constraints and dampened multi-step predictions
- Model selection framework with `--model` flag supporting `baseline` (default) and `arima`
- Model factory pattern in forecaster for dynamic model instantiation
- ARIMA deployment example in `examples/deployment-arima.yaml` with recommended configuration
- Comprehensive model selection guide in `docs/model-selection.md` covering:
  - When to use each model (baseline vs ARIMA)
  - ARIMA parameter tuning guidelines (p, d, q explained)
  - Performance benchmarks and accuracy comparisons
  - Troubleshooting guide and best practices
- Security audit documentation in `docs/SECURITY_AUDIT.md` with 18 identified findings

### Changed

- Updated README.md with forecasting models comparison table
- Model initialization now uses factory pattern for extensibility
- Forecaster configuration extended with model selection parameters
- Enhanced logging for model initialization with parameter details

### Security

- Added GitHub Actions workflow for gosec security scanning
  - Automated SARIF report upload to GitHub Code Scanning
  - Configured to exclude auto-generated protobuf files
  - Runs on push and weekly schedule
- Added workflow permissions for security events and code scanning
- Documented 18 security findings (3 critical, 4 high, 6 medium, 5 low) for future remediation

### Documentation

- Added detailed ARIMA implementation documentation
- Created comprehensive model selection and tuning guide
- Added ARIMA deployment example with resource requirements
- Documented model comparison (algorithm, training, accuracy, use cases)
- Added benchmark results for ARIMA training and prediction performance

### Performance

- ARIMA training: ~15μs per 1000 data points
- ARIMA prediction: <1μs for 30-step forecast
- Memory overhead: ~5MB for ARIMA vs ~1MB for baseline
- All unit tests and benchmarks passing

## [0.1.1] - 2025-12-12

### Added

- **Redis Storage Backend**: Added Redis as an optional storage backend for forecast snapshots, enabling multi-instance deployments and persistent storage
  - New `pkg/storage/redis.go` implementing the Store interface with Redis
  - Comprehensive test suite with 14 tests using testcontainers for integration testing
  - Configuration flags: `--storage`, `--redis-addr`, `--redis-password`, `--redis-db`, `--redis-ttl`
  - TTL-based expiration, connection pooling, and automatic health checks
  - Idempotent `Close()` implementation for graceful shutdown
- Redis deployment example in `examples/deployment-redis.yaml` showing HA setup with 2 forecaster replicas
- Storage backend factory in `cmd/forecaster/store` package with fail-fast initialization
- Package-level documentation for all forecaster and scaler packages (godoc/pkg.go.dev compatible)
- Comprehensive function headers following Go documentation conventions

### Changed

- Forecaster now uses pointer type for `capacity.Policy` for consistency
- Storage initialization moved to dedicated factory package
- Updated README.md with storage backends comparison table and usage examples
- Improved error handling and logging throughout storage initialization

### Fixed

- Fixed store cleanup to prevent premature connection closing
- Fixed type consistency between Forecaster struct and constructor

### Dependencies

- Added `github.com/redis/go-redis/v9` v9.7.0
- Added `github.com/testcontainers/testcontainers-go/modules/redis` v0.40.0
- Bumped `golang.org/x/crypto` from 0.43.0 to 0.45.0

### Documentation

- Added detailed package documentation for `cmd/forecaster/store`
- Added function headers for all exported functions in forecaster and scaler packages
- Updated README with storage backend section
- Added Redis deployment example with annotations

## [0.1.0] - 2024-12-09

### Added

- Initial release of Kedastral predictive autoscaler
- Baseline forecasting model with trend detection
- Prometheus adapter for metrics collection
- Capacity planning with configurable policies
- HTTP API for forecast snapshots
- KEDA External Scaler integration
- In-memory storage for single-instance deployments
- Comprehensive test suite
- Kubernetes deployment examples
- Documentation and getting started guide

[0.1.5]: https://github.com/kedastral/kedastral/compare/v0.1.4...v0.1.5
[0.1.4]: https://github.com/kedastral/kedastral/compare/v0.1.3...v0.1.4
[0.1.3]: https://github.com/kedastral/kedastral/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/kedastral/kedastral/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/kedastral/kedastral/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/kedastral/kedastral/releases/tag/v0.1.0
