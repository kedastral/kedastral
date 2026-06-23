# Contributing to Kedastral

Thanks for your interest in contributing! Kedastral is an open-source predictive
autoscaling companion for [KEDA](https://keda.sh/), and contributions of all kinds —
bug reports, fixes, features, docs, and examples — are welcome.

## Getting Started

### Prerequisites

- Go 1.26 or later
- `make`
- Docker (for building images and running integration tests)
- Optional: `golangci-lint`, `helm`, `kind`/`minikube`, `kubectl`

### Build and test

```bash
make build           # build forecaster, scaler, mcp-server into bin/
make test            # run all unit tests
make test-coverage   # run tests with an HTML coverage report
make lint            # run golangci-lint
make fmt             # gofmt the codebase
make tidy            # go mod tidy
```

Please run `make fmt`, `make test`, and (if available) `make lint` before opening a
pull request.

## Development Workflow

1. Fork the repository and create a branch off `main`.
2. Make focused changes with clear, minimal diffs.
3. Add or update tests for any behavior change. Prefer fast, table-driven unit tests.
4. Keep the docs in sync — if you change flags, CRDs, or behavior, update the relevant
   files under `docs/` and `README.md`.
5. Run `make fmt test lint` locally.
6. Open a pull request describing the change and the motivation.

### Commit messages

This project uses [Conventional Commits](https://www.conventionalcommits.org/), e.g.:

```
feat: add VictoriaMetrics adapter
fix: clamp scale-down to DownMaxPercentPerStep
docs: document operator mode in QUICKSTART
chore(deps): bump grpc to 1.79.3
```

## Code Standards

- Follow the conventions in [`.claude/CLAUDE.md`](.claude/CLAUDE.md), which is the
  source of truth for repository structure and coding standards.
- Use `context.Context` as the first argument for I/O and long-running operations.
- Wrap errors with `%w`; check with `errors.Is`/`errors.As`. No panics in libraries.
- Use structured logging via `log/slog`.
- Keep functions small and testable; prefer pure functions in `pkg/capacity` and
  `pkg/models`.
- Don't add third-party dependencies without discussion.

## Working on the Operator / CRDs

The API types live in `pkg/api/v1alpha1`. After editing them, regenerate the
generated code and manifests:

```bash
make generate    # deepcopy methods (zz_generated.deepcopy.go)
make manifests   # CRD YAML into deploy/helm/kedastral/crds/
```

Commit the regenerated files alongside your changes. See [`docs/OPERATOR.md`](docs/OPERATOR.md)
for the operator architecture.

## Regenerating gRPC code

The KEDA external scaler protobufs are regenerated with:

```bash
make proto
```

## Reporting Bugs and Requesting Features

- Open a [GitHub Issue](https://github.com/kedastral/kedastral/issues) with a clear
  description, reproduction steps, and your environment (Kubernetes/KEDA versions).
- For questions and ideas, use
  [GitHub Discussions](https://github.com/kedastral/kedastral/discussions).

## Ways to Contribute

- Add new adapters (Kafka, CloudWatch, Datadog, custom HTTP sources)
- Add or improve forecasting models
- Improve documentation and examples
- Triage issues and review pull requests

## License

By contributing, you agree that your contributions will be licensed under the
[MIT License](LICENSE).
