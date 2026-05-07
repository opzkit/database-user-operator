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
# Skip when CI has already built+loaded the image via buildx with GHA cache.
cd "${PROJECT_ROOT}"
if [[ "${SKIP_OPERATOR_BUILD:-}" == "true" ]]; then
    echo "Skipping operator image build (SKIP_OPERATOR_BUILD=true)"
    if ! docker image inspect database-user-operator:test >/dev/null 2>&1; then
        echo "ERROR: database-user-operator:test not present locally — build it before invoking this script."
        exit 1
    fi
else
    echo "Building operator image with coverage..."
    docker build --build-arg ENABLE_COVERAGE=true -t database-user-operator:test .
fi

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

# Deploy all services in parallel
echo "Deploying PostgreSQL (incl. version matrix 14/15/16/17/18), MySQL, LocalStack..."
kubectl apply -f "${PROJECT_ROOT}/test/integration/manifests/postgres.yaml" &
kubectl apply -f "${PROJECT_ROOT}/test/integration/manifests/postgres-versions.yaml" &
kubectl apply -f "${PROJECT_ROOT}/test/integration/manifests/mysql.yaml" &
kubectl apply -f "${PROJECT_ROOT}/test/integration/manifests/localstack.yaml" &
wait

# Wait for deployments in parallel
echo ""
echo "Waiting for all deployments to be available..."
LOG_DIR=$(mktemp -d -t deploy-wait.XXXXXX)
trap 'rm -rf "${LOG_DIR}"' EXIT

declare -a WAIT_PIDS
declare -a WAIT_NAMES

start_wait() {
    local name=$1
    local ns=$2
    wait_for_deployment "${name}" "${ns}" 300 > "${LOG_DIR}/${name}.log" 2>&1 &
    WAIT_PIDS+=("$!")
    WAIT_NAMES+=("${name}")
}

start_wait postgres databases
for v in 14 15 16 17 18; do
    start_wait "postgres-${v}" databases
done
start_wait mysql databases
start_wait localstack databases

failed=0
for i in "${!WAIT_PIDS[@]}"; do
    name=${WAIT_NAMES[$i]}
    pid=${WAIT_PIDS[$i]}
    if wait "${pid}"; then
        echo "  ✓ ${name}"
    else
        echo "  ✗ ${name} failed:"
        cat "${LOG_DIR}/${name}.log"
        failed=$((failed + 1))
    fi
done

if [[ ${failed} -gt 0 ]]; then
    echo "ERROR: ${failed} deployment(s) failed"
    exit 1
fi

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
