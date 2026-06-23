#!/usr/bin/env bash
#
# One-command Kedastral operator demo on kind.
#
# Brings up a kind cluster with KEDA, Prometheus, a sample app + load generator, and
# Kedastral in operator mode, then declares a DataSource + ForecastPolicy so Kedastral
# forecasts the app's request rate and generates a KEDA ScaledObject to scale it.

set -euo pipefail

CLUSTER=kedastral-demo
NS=kedastral-demo
TAG=demo
REPO_ROOT=$(cd "$(dirname "$0")/../.." && pwd)
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)

green() { printf '\033[0;32m%s\033[0m\n' "$1"; }

echo "Checking prerequisites..."
for cmd in docker kind kubectl helm; do
  command -v "$cmd" >/dev/null 2>&1 || { echo "Error: $cmd is required but not installed." >&2; exit 1; }
done
green "✓ prerequisites found"

if kind get clusters 2>/dev/null | grep -qx "$CLUSTER"; then
  green "✓ kind cluster '$CLUSTER' already exists"
else
  echo "Creating kind cluster '$CLUSTER'..."
  kind create cluster --name "$CLUSTER"
fi

echo "Building images..."
cd "$REPO_ROOT"
docker build -q -t "kedastral-forecaster:$TAG" -f Dockerfile.forecaster . >/dev/null
docker build -q -t "kedastral-scaler:$TAG" -f Dockerfile.scaler . >/dev/null
docker build -q -t "simple-app-cmd:$TAG" examples/simple-app/cmd >/dev/null
docker build -q -t "load-generator:$TAG" examples/simple-app/load-generator >/dev/null
green "✓ images built"

echo "Loading images into kind..."
for img in "kedastral-forecaster:$TAG" "kedastral-scaler:$TAG" "simple-app-cmd:$TAG" "load-generator:$TAG"; do
  kind load docker-image "$img" --name "$CLUSTER"
done
green "✓ images loaded"

echo "Installing KEDA..."
helm repo add kedacore https://kedacore.github.io/charts >/dev/null 2>&1 || true
helm repo update >/dev/null
helm upgrade --install keda kedacore/keda --namespace keda --create-namespace --wait
green "✓ KEDA installed"

echo "Deploying Prometheus, sample app, and load generator..."
kubectl apply -f "$SCRIPT_DIR/manifests/00-namespace.yaml"
kubectl apply -f "$SCRIPT_DIR/manifests/01-prometheus.yaml"
kubectl apply -f "$SCRIPT_DIR/manifests/02-app.yaml"
kubectl apply -f "$SCRIPT_DIR/manifests/03-load-generator.yaml"
green "✓ workload deployed"

echo "Installing Kedastral (operator mode)..."
helm upgrade --install kedastral "$REPO_ROOT/deploy/helm/kedastral" \
  --namespace "$NS" \
  --set forecaster.operator.enabled=true \
  --set "forecaster.operator.scalerAddress=kedastral-scaler.$NS:50051" \
  --set forecaster.image.repository=kedastral-forecaster \
  --set forecaster.image.tag="$TAG" \
  --set forecaster.image.pullPolicy=IfNotPresent \
  --set scaler.image.repository=kedastral-scaler \
  --set scaler.image.tag="$TAG" \
  --set scaler.image.pullPolicy=IfNotPresent \
  --wait
green "✓ Kedastral installed"

echo "Applying DataSource and ForecastPolicy..."
kubectl apply -f "$SCRIPT_DIR/datasource.yaml"
kubectl apply -f "$SCRIPT_DIR/forecastpolicy.yaml"
green "✓ policy applied"

echo ""
green "========================================="
green "Demo ready!"
green "========================================="
cat <<EOF

# Watch the policy status, generated ScaledObject, HPA, and pods:
  kubectl get forecastpolicy,scaledobject,hpa,pods -n $NS

# Inspect the policy (current/desired replicas, last forecast time):
  kubectl describe forecastpolicy simple-app -n $NS

# Forecaster logs:
  kubectl logs -f -n $NS -l app.kubernetes.io/component=forecaster

# Current forecast snapshot:
  kubectl exec -n $NS deploy/kedastral-forecaster -- \\
    wget -qO- "http://localhost:8081/forecast/current?workload=$NS-simple-app"

# Try a different load pattern:
  kubectl set env deploy/load-generator -n $NS PATTERN=double-peak

# Tear everything down:
  $SCRIPT_DIR/cleanup.sh
EOF
