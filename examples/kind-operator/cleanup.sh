#!/usr/bin/env bash
#
# Tears down the Kedastral operator demo by deleting the kind cluster.

set -euo pipefail

CLUSTER=kedastral-demo

if ! kind get clusters 2>/dev/null | grep -qx "$CLUSTER"; then
  echo "kind cluster '$CLUSTER' not found, nothing to do."
  exit 0
fi

read -r -p "Delete kind cluster '$CLUSTER' and everything in it? [y/N] " reply
case "$reply" in
  y | Y | yes | YES)
    kind delete cluster --name "$CLUSTER"
    echo "Deleted kind cluster '$CLUSTER'."
    ;;
  *)
    echo "Aborted."
    ;;
esac
