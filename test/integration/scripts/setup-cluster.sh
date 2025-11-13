#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

CLUSTER_NAME="${CLUSTER_NAME:-database-operator-test}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"

# Prefer tools from bin directory if they exist
LOCALBIN="${PROJECT_ROOT}/bin"
if [ -d "$LOCALBIN" ]; then
    export PATH="${LOCALBIN}:${PATH}"
fi

# Verify required tools are available
command -v kubectl >/dev/null 2>&1 || { echo "ERROR: kubectl not found. Run 'make integration-tools' first."; exit 1; }
command -v kind >/dev/null 2>&1 || { echo "ERROR: kind not found. Run 'make integration-tools' first."; exit 1; }
command -v helm >/dev/null 2>&1 || { echo "ERROR: helm not found. Run 'make integration-tools' first."; exit 1; }

echo "===> Setting up kind cluster: ${CLUSTER_NAME}"
echo "Using tools:"
echo "  - kubectl: $(which kubectl)"
echo "  - kind: $(which kind)"
echo "  - helm: $(which helm)"
echo ""

# Build operator image first - fail fast if build issues
echo "Building operator image with coverage..."
cd "${PROJECT_ROOT}"
docker build --build-arg ENABLE_COVERAGE=true -t database-user-operator:test .

# Prepare Helm chart with CRDs
echo "Preparing Helm chart with CRDs..."
make helm-crds

# Create coverage directory on host with proper permissions
mkdir -p /tmp/coverage
chmod 777 /tmp/coverage 2>/dev/null || echo "Warning: Could not set permissions on /tmp/coverage (non-fatal)"

echo ""
echo "✓ Operator image built successfully"
echo ""

# Check if cluster already exists
if kind get clusters | grep -q "^${CLUSTER_NAME}$"; then
    echo "Cluster ${CLUSTER_NAME} already exists, deleting..."
    kind delete cluster --name="${CLUSTER_NAME}"
fi

# Create kind cluster
echo "Creating kind cluster..."
kind create cluster \
    --name="${CLUSTER_NAME}" \
    --config="${PROJECT_ROOT}/test/integration/kind-config.yaml" \
    --wait=5m

# Wait for cluster to be ready
echo "Waiting for cluster to be ready..."
kubectl wait --for=condition=Ready nodes --all --timeout=5m

# Create namespaces
echo "Creating namespaces..."
kubectl create namespace databases || true
kubectl create namespace db-system || true

# Function to wait for deployment with minimal output
wait_for_deployment() {
    local deployment=$1
    local namespace=$2
    local timeout=${3:-300}  # Default 5 minutes

    echo -n "Waiting for deployment/${deployment}..."

    # Wait for deployment to be available
    if kubectl wait --for=condition=available --timeout=${timeout}s deployment/${deployment} -n ${namespace} 2>/dev/null; then
        echo " ✓"
        return 0
    fi

    # If we reach here, deployment failed
    echo " ✗"
    echo ""
    echo "ERROR: Timeout waiting for deployment/${deployment} in namespace ${namespace}"
    echo ""
    echo "=== Pod Status ==="
    kubectl get pods -n ${namespace} -l app=${deployment}
    echo ""
    echo "=== Pod Description ==="
    kubectl describe pods -n ${namespace} -l app=${deployment}
    echo ""
    echo "=== Pod Logs ==="
    local pod_name=$(kubectl get pods -n ${namespace} -l app=${deployment} -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
    if [ -n "$pod_name" ]; then
        kubectl logs -n ${namespace} ${pod_name} --all-containers --tail=100 2>/dev/null || echo "(No logs available)"
    else
        echo "(No pods found)"
    fi
    echo ""
    return 1
}

# Deploy all services
echo "Deploying PostgreSQL..."
kubectl apply -f "${PROJECT_ROOT}/test/integration/manifests/postgres.yaml"

echo "Deploying MySQL..."
kubectl apply -f "${PROJECT_ROOT}/test/integration/manifests/mysql.yaml"

echo "Deploying LocalStack..."
kubectl apply -f "${PROJECT_ROOT}/test/integration/manifests/localstack.yaml"

# Wait for all deployments to be available with periodic logging
echo ""
echo "Waiting for all deployments to be available..."
wait_for_deployment postgres databases 300
wait_for_deployment mysql databases 300
wait_for_deployment localstack databases 300

echo ""
echo "✓ All services are ready"

# Initialize LocalStack with secrets
echo "Initializing LocalStack with test secrets..."
chmod +x "${PROJECT_ROOT}/test/integration/scripts/init-localstack.sh"
"${PROJECT_ROOT}/test/integration/scripts/init-localstack.sh"

# Load operator image into kind
echo "Loading operator image into kind..."
kind load docker-image database-user-operator:test --name="${CLUSTER_NAME}"

# Deploy operator with coverage configuration
echo "Deploying operator..."
helm install database-user-operator "${PROJECT_ROOT}/helm/database-user-operator" \
    --namespace db-system \
    --set image.repository=database-user-operator \
    --set image.tag=test \
    --set image.pullPolicy=Never \
    --set-json 'env=[
        {"name":"AWS_ACCESS_KEY_ID","value":"test"},
        {"name":"AWS_SECRET_ACCESS_KEY","value":"test"},
        {"name":"AWS_REGION","value":"us-east-1"},
        {"name":"AWS_ENDPOINT_URL","value":"http://localstack.databases.svc.cluster.local:4566"},
        {"name":"GOCOVERDIR","value":"/tmp/coverage"}
    ]' \
    --set-json 'extraVolumes=[{"name":"coverage","hostPath":{"path":"/tmp/coverage","type":"DirectoryOrCreate"}}]' \
    --set-json 'extraVolumeMounts=[{"name":"coverage","mountPath":"/tmp/coverage"}]' \
    --wait \
    --timeout=5m

echo "===> Cluster setup complete!"
echo ""
echo "Cluster info:"
kubectl cluster-info --context "kind-${CLUSTER_NAME}"
echo ""
echo "Running pods:"
kubectl get pods -A
