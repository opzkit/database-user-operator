#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

CLUSTER_NAME="${CLUSTER_NAME:-database-operator-test}"

echo "===> Tearing down kind cluster: ${CLUSTER_NAME}"

if kind get clusters | grep -q "^${CLUSTER_NAME}$"; then
    kind delete cluster --name="${CLUSTER_NAME}"
    echo "Cluster ${CLUSTER_NAME} deleted"
else
    echo "Cluster ${CLUSTER_NAME} does not exist"
fi
