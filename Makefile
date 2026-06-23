.PHONY: all build test clean proto help forecaster scaler mcp-server backtest generate manifests

# Version can be set via environment variable or defaults to dev
VERSION ?= dev
LDFLAGS := -X main.version=$(VERSION)

# controller-gen is used to generate deepcopy methods and CRD manifests
CONTROLLER_GEN_VERSION ?= v0.21.0
CONTROLLER_GEN := go run sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)
CRD_OUTPUT_DIR := deploy/helm/kedastral/crds

# Default target
all: build

# Build all executables
build: forecaster scaler mcp-server backtest

# Build forecaster
forecaster:
	@echo "Building forecaster..."
	@go build -ldflags "$(LDFLAGS)" -o bin/forecaster ./cmd/forecaster

# Build scaler
scaler:
	@echo "Building scaler..."
	@go build -ldflags "$(LDFLAGS)" -o bin/scaler ./cmd/scaler

# Build MCP server
mcp-server:
	@echo "Building mcp-server..."
	@go build -ldflags "$(LDFLAGS)" -o bin/mcp-server ./cmd/mcp-server

# Build backtest tool
backtest:
	@echo "Building backtest..."
	@go build -ldflags "$(LDFLAGS)" -o bin/backtest ./cmd/backtest

# Run all tests
test:
	@echo "Running tests..."
	@go test ./... -v

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	@go test ./... -coverprofile=coverage.out
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Generate protobuf code
proto:
	@echo "Generating protobuf code..."
	@cd pkg/api/externalscaler && protoc \
		--go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		externalscaler.proto
	@echo "Protobuf code generated"

# Generate deepcopy methods for API types
generate:
	@echo "Generating deepcopy methods..."
	@$(CONTROLLER_GEN) object:headerFile=hack/boilerplate.go.txt paths=./pkg/api/...

# Generate CRD manifests into the Helm chart
manifests:
	@echo "Generating CRD manifests..."
	@$(CONTROLLER_GEN) crd:allowDangerousTypes=true paths=./pkg/api/... output:crd:dir=$(CRD_OUTPUT_DIR)

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/
	@rm -f coverage.out coverage.html
	@echo "Clean complete"

# Install dev dependencies
install-tools:
	@echo "Installing development tools..."
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@echo "Tools installed"

# Run forecaster (for local dev)
run-forecaster:
	@echo "Running forecaster..."
	@go run ./cmd/forecaster \
		-workload=demo-api \
		-metric=http_rps \
		-prom-query='sum(rate(http_requests_total[1m]))' \
		-prom-url=http://localhost:9090 \
		-log-level=debug

# Run scaler (for local dev)
run-scaler:
	@echo "Running scaler..."
	@go run ./cmd/scaler \
		-forecaster-url=http://localhost:8081 \
		-log-level=debug

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# Run linter
lint:
	@echo "Running linter..."
	@golangci-lint run

# Tidy dependencies
tidy:
	@echo "Tidying dependencies..."
	@go mod tidy

# Help
help:
	@echo "Kedastral Makefile targets:"
	@echo ""
	@echo "  make build           - Build all binaries (forecaster, scaler, mcp-server)"
	@echo "  make forecaster      - Build forecaster binary only"
	@echo "  make scaler          - Build scaler binary only"
	@echo "  make mcp-server      - Build mcp-server binary only"
	@echo "  make backtest        - Build backtest tool only"
	@echo "  make test            - Run all tests"
	@echo "  make test-coverage   - Run tests with coverage report"
	@echo "  make proto           - Regenerate protobuf code"
	@echo "  make generate        - Regenerate API deepcopy methods"
	@echo "  make manifests       - Regenerate CRD manifests into the Helm chart"
	@echo "  make clean           - Remove build artifacts"
	@echo "  make install-tools   - Install development tools"
	@echo "  make run-forecaster  - Run forecaster locally"
	@echo "  make run-scaler      - Run scaler locally"
	@echo "  make fmt             - Format code"
	@echo "  make lint            - Run linter"
	@echo "  make tidy            - Tidy Go modules"
	@echo "  make help            - Show this help message"
