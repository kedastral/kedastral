#!/bin/bash

set -e

echo "========================================="
echo "Kedastral Test Environment Setup"
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
# First wait for the Prometheus pod to be created (can take time for operator to create it)
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
# Now wait for it to be ready
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=prometheus -n monitoring --timeout=300s
echo -e "${GREEN}✓ Prometheus is ready${NC}"
echo ""

# Build Kedastral images
echo "Building Kedastral Docker images..."
cd ..
echo "Building forecaster..."
docker build -t kedastral-forecaster:v2 -f Dockerfile.forecaster .
echo "Building scaler..."
docker build -t kedastral-scaler:latest -f Dockerfile.scaler .
cd examples/simple-app
echo -e "${GREEN}✓ Kedastral images built${NC}"
echo ""

# Build test app images
echo "Building test application images..."
echo "Building app..."
cd cmd
docker build -t simple-app:latest .
cd ..

echo "Building load generator..."
cd load-generator
docker build -t load-generator:latest .
cd ..
echo -e "${GREEN}✓ Test app images built${NC}"
echo ""

# Deploy test application
echo "Deploying test application..."
kubectl apply -f k8s/01-namespace.yaml
kubectl apply -f k8s/02-database.yaml

# Wait for postgres to be ready
echo "Waiting for PostgreSQL to be ready..."
kubectl wait --for=condition=ready pod -l app=postgres -n simple-app --timeout=120s

kubectl apply -f k8s/03-backend.yaml
kubectl apply -f k8s/04-load-generator.yaml
echo -e "${GREEN}✓ Test application deployed${NC}"
echo ""

# Wait for test app to be ready
echo "Waiting for app to be ready..."
kubectl wait --for=condition=ready pod -l app=simple-app -n simple-app --timeout=120s
echo -e "${GREEN}✓ Test app is ready${NC}"
echo ""

# Deploy Kedastral
echo "Deploying Kedastral..."
kubectl apply -f k8s/05-kedastral-forecaster.yaml
kubectl apply -f k8s/06-kedastral-scaler.yaml

# Wait for Kedastral to be ready
echo "Waiting for Kedastral to be ready..."
kubectl wait --for=condition=ready pod -l app=kedastral-forecaster -n simple-app --timeout=120s
kubectl wait --for=condition=ready pod -l app=kedastral-scaler -n simple-app --timeout=120s
echo -e "${GREEN}✓ Kedastral is ready${NC}"
echo ""

# Deploy KEDA ScaledObject
echo "Deploying KEDA ScaledObject..."
kubectl apply -f k8s/07-scaled-object.yaml
echo -e "${GREEN}✓ ScaledObject deployed${NC}"
echo ""

echo -e "${GREEN}========================================="
echo "Setup Complete!"
echo "=========================================${NC}"
echo ""
echo "Useful commands:"
echo ""
echo "# Watch pod scaling in real-time:"
echo "  kubectl get pods -n simple-app -w"
echo ""
echo "# Check KEDA HPA status:"
echo "  kubectl get hpa -n simple-app"
echo ""
echo "# View Kedastral forecaster logs:"
echo "  kubectl logs -f -l app=kedastral-forecaster -n simple-app"
echo ""
echo "# View Kedastral scaler logs:"
echo "  kubectl logs -f -l app=kedastral-scaler -n simple-app"
echo ""
echo "# Check current forecast:"
echo "  kubectl exec -n simple-app deploy/kedastral-forecaster -- wget -qO- http://localhost:8081/forecast/current?workload=simple-app"
echo ""
echo "# Access Prometheus UI:"
echo "  kubectl port-forward -n monitoring svc/prometheus-operated 9090:9090"
echo "  # Then open http://localhost:9090"
echo ""
echo "# Access Grafana UI:"
echo "  kubectl port-forward -n monitoring svc/prometheus-grafana 3000:80"
echo "  # Then open http://localhost:3000 (admin/prom-operator)"
echo ""
echo "# View load generator pattern:"
echo "  kubectl logs -f -l app=load-generator -n simple-app"
echo ""
echo "# Change load pattern (edit deployment):"
echo "  # Options: constant, hourly-spike, business-hours, sine-wave, double-peak"
echo "  kubectl set env deployment/load-generator PATTERN=sine-wave -n simple-app"
echo ""
echo "# Test the app directly:"
echo "  kubectl port-forward -n simple-app svc/simple-app 8080:8080"
echo "  # Then: curl http://localhost:8080"
echo ""
echo "# Cleanup everything:"
echo "  ./cleanup.sh"
echo ""
