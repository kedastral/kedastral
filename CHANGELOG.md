# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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

[0.1.3]: https://github.com/kedastral/kedastral/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/kedastral/kedastral/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/kedastral/kedastral/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/kedastral/kedastral/releases/tag/v0.1.0
