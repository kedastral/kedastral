#!/bin/bash

echo "Cleaning up Kedastral multi-app Redis example..."

# Delete namespace (this removes everything in it)
kubectl delete namespace multi-app-redis --ignore-not-found

# Optionally delete KEDA and Prometheus
read -p "Delete KEDA? (y/N) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    helm uninstall keda -n keda-system
    kubectl delete namespace keda-system
fi

read -p "Delete Prometheus? (y/N) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    helm uninstall prometheus -n monitoring
    kubectl delete namespace monitoring
fi

read -p "Stop minikube? (y/N) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    minikube stop
fi

echo "Cleanup complete!"
