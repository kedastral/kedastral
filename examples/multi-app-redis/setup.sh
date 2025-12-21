#!/bin/bash

set -e

echo "========================================="
echo "Kedastral Multi-App Redis Example Setup"
echo "========================================="
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check prerequisites
echo "Checking prerequisites..."
command -v docker >/dev/null 2>&1 || { echo -e "${RED}Error: docker is required but not installed.${NC}" >&2; exit 1; }
command -v minikube >/dev/null 2>&1 || { echo -e "${RED}Error: minikube is required but not installed.${NC}" >&2; exit 1; }
command -v kubectl >/dev/null 2>&1 || { echo -e "${RED}Error: kubectl is required but not installed.${NC}" >&2; exit 1; }
command -v helm >/dev/null 2>&1 || { echo -e "${RED}Error: helm is required but not installed.${NC}" >&2; exit 1; }
echo -e "${GREEN}✓ All prerequisites found${NC}"
echo ""

# Start minikube if not running
echo "Checking minikube status..."
if ! minikube status &>/dev/null; then
    echo "Starting minikube..."
    minikube start --cpus=4 --memory=7168 --driver=docker
else
    echo -e "${GREEN}✓ Minikube is already running${NC}"
fi
echo ""

# Set docker env to use minikube's docker daemon
echo "Configuring Docker to use minikube's daemon..."
eval $(minikube -p minikube docker-env)
echo -e "${GREEN}✓ Docker configured${NC}"
echo ""

# Install KEDA
echo "Installing KEDA..."
if kubectl get namespace keda-system &>/dev/null; then
    echo -e "${YELLOW}KEDA namespace already exists, skipping...${NC}"
else
    helm repo add kedacore https://kedacore.github.io/charts
    helm repo update
    helm install keda kedacore/keda --namespace keda-system --create-namespace
    echo -e "${GREEN}✓ KEDA installed${NC}"
fi
echo ""

# Install Prometheus
echo "Installing Prometheus (kube-prometheus-stack)..."
if kubectl get namespace monitoring &>/dev/null; then
    echo -e "${YELLOW}Monitoring namespace already exists, skipping...${NC}"
else
    helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
    helm repo update
    helm install prometheus prometheus-community/kube-prometheus-stack \
        --namespace monitoring \
        --create-namespace \
        --set prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues=false \
        --set prometheus.prometheusSpec.podMonitorSelectorNilUsesHelmValues=false
    echo -e "${GREEN}✓ Prometheus installed${NC}"
fi
echo ""

# Wait for Prometheus to be ready
echo "Waiting for Prometheus to be ready..."
echo "Waiting for Prometheus pod to be created..."
for i in {1..60}; do
    POD_COUNT=$(kubectl get pod -l app.kubernetes.io/name=prometheus -n monitoring --no-headers 2>/dev/null | wc -l)
    if [ "$POD_COUNT" -gt 0 ]; then
        echo "Prometheus pod found, waiting for it to be ready..."
        break
    fi
    if [ $i -eq 60 ]; then
        echo -e "${RED}Error: Prometheus pod was not created after 5 minutes${NC}"
        exit 1
    fi
    sleep 5
done
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=prometheus -n monitoring --timeout=300s
echo -e "${GREEN}✓ Prometheus is ready${NC}"
echo ""

# Build Kedastral images
echo "Building Kedastral Docker images..."
cd ../..
echo "Building forecaster..."
docker build -t kedastral-forecaster:v2 -f Dockerfile.forecaster .
echo "Building scaler..."
docker build -t kedastral-scaler:latest -f Dockerfile.scaler .
cd examples/multi-app-redis
echo -e "${GREEN}✓ Kedastral images built${NC}"
echo ""

# Build application images
echo "Building application images..."
echo "Building API app..."
cd cmd
docker build -t multi-app-api:latest .
echo "Building Worker app..."
docker build -t multi-app-worker:latest .
cd ..

echo "Building load generator..."
cd load-generator
docker build -t load-generator:latest .
cd ..
echo -e "${GREEN}✓ Application images built${NC}"
echo ""

# Deploy namespace and Redis
echo "Deploying namespace and Redis..."
kubectl apply -f k8s/01-namespace.yaml
kubectl apply -f k8s/02-redis.yaml

# Wait for Redis to be ready
echo "Waiting for Redis to be ready..."
kubectl wait --for=condition=ready pod -l app=redis -n multi-app-redis --timeout=120s
echo -e "${GREEN}✓ Redis is ready${NC}"
echo ""

# Deploy applications
echo "Deploying applications..."
kubectl apply -f k8s/03-api-backend.yaml
kubectl apply -f k8s/04-worker-backend.yaml
kubectl apply -f k8s/05-api-load-generator.yaml
kubectl apply -f k8s/06-worker-load-generator.yaml

# Wait for apps to be ready
echo "Waiting for applications to be ready..."
kubectl wait --for=condition=ready pod -l app=api-app -n multi-app-redis --timeout=120s
kubectl wait --for=condition=ready pod -l app=worker-app -n multi-app-redis --timeout=120s
echo -e "${GREEN}✓ Applications are ready${NC}"
echo ""

# Deploy Kedastral
echo "Deploying Kedastral forecasters and scaler..."
kubectl apply -f k8s/07-kedastral-forecaster-api.yaml
kubectl apply -f k8s/08-kedastral-forecaster-worker.yaml
kubectl apply -f k8s/09-kedastral-scaler.yaml

# Wait for Kedastral to be ready
echo "Waiting for Kedastral to be ready..."
kubectl wait --for=condition=ready pod -l app=kedastral-forecaster-api -n multi-app-redis --timeout=120s
kubectl wait --for=condition=ready pod -l app=kedastral-forecaster-worker -n multi-app-redis --timeout=120s
kubectl wait --for=condition=ready pod -l app=kedastral-scaler -n multi-app-redis --timeout=120s
echo -e "${GREEN}✓ Kedastral is ready${NC}"
echo ""

# Deploy KEDA ScaledObjects
echo "Deploying KEDA ScaledObjects..."
kubectl apply -f k8s/10-scaled-object-api.yaml
kubectl apply -f k8s/11-scaled-object-worker.yaml
echo -e "${GREEN}✓ ScaledObjects deployed${NC}"
echo ""

echo -e "${GREEN}========================================="
echo "Setup Complete!"
echo "=========================================${NC}"
echo ""
echo "This example demonstrates:"
echo "  • Multiple workloads (API + Worker) with different patterns"
echo "  • Shared Redis storage for forecasts"
echo "  • Two forecasters writing to Redis"
echo "  • Single scaler reading from Redis for both workloads"
echo ""
echo "Useful commands:"
echo ""
echo "# Watch all pods:"
echo "  kubectl get pods -n multi-app-redis -w"
echo ""
echo "# Check both HPA statuses:"
echo "  kubectl get hpa -n multi-app-redis"
echo ""
echo "# View API forecaster logs:"
echo "  kubectl logs -f -l app=kedastral-forecaster-api -n multi-app-redis"
echo ""
echo "# View Worker forecaster logs:"
echo "  kubectl logs -f -l app=kedastral-forecaster-worker -n multi-app-redis"
echo ""
echo "# View scaler logs:"
echo "  kubectl logs -f -l app=kedastral-scaler -n multi-app-redis"
echo ""
echo "# Check API forecast:"
echo "  kubectl exec -n multi-app-redis deploy/kedastral-scaler -- wget -qO- http://kedastral-forecaster-api:8081/forecast/current?workload=api-app"
echo ""
echo "# Check Worker forecast:"
echo "  kubectl exec -n multi-app-redis deploy/kedastral-scaler -- wget -qO- http://kedastral-forecaster-worker:8081/forecast/current?workload=worker-app"
echo ""
echo "# Inspect Redis keys:"
echo "  kubectl exec -n multi-app-redis deploy/redis -- redis-cli KEYS '*'"
echo ""
echo "# View forecast data in Redis:"
echo "  kubectl exec -n multi-app-redis deploy/redis -- redis-cli GET forecast:api-app"
echo "  kubectl exec -n multi-app-redis deploy/redis -- redis-cli GET forecast:worker-app"
echo ""
echo "# Change load patterns:"
echo "  kubectl set env deployment/api-load-generator PATTERN=sine-wave -n multi-app-redis"
echo "  kubectl set env deployment/worker-load-generator PATTERN=double-peak -n multi-app-redis"
echo ""
echo "# Cleanup:"
echo "  ./cleanup.sh"
echo ""
