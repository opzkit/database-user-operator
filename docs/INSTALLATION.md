# Installation Guide

This guide covers installing and configuring the Database User Operator in your Kubernetes cluster.

## Prerequisites

### Required
- Kubernetes cluster version 1.28 or higher
- kubectl configured to access your cluster
- Cluster admin permissions (for CRD installation)
- PostgreSQL database instance (self-hosted or RDS)
- AWS account with Secrets Manager access
- AWS credentials configured (see [AWS Credentials](AWS_CREDENTIALS.md))

### Recommended
- Helm 3.x (for Helm-based installation)
- make and Go 1.22+ (for building from source)

## Installation Methods

### Method 1: Using Helm (Recommended)

The Helm chart is the recommended installation method for production use.

1. Install the operator:
```bash
helm install database-user-operator ./helm/database-user-operator \
  --namespace db-system \
  --create-namespace
```

2. Or with custom values:
```bash
helm install database-user-operator ./helm/database-user-operator \
  --namespace db-system \
  --create-namespace \
  --values custom-values.yaml
```

See [Configuration](#configuration) below for customization options.

### Method 2: Using Make (Development)

1. Clone the repository:
```bash
git clone https://github.com/opzkit/database-user-operator.git
cd database-user-operator
```

2. Install CRDs:
```bash
make install
```

3. Deploy the operator:
```bash
make deploy IMG=ghcr.io/opzkit/database-user-operator:latest
```

4. Verify installation:
```bash
kubectl get deployment -n database-user-operator-system
kubectl get pods -n database-user-operator-system
```

### Method 3: Using Kustomize

For advanced customization:
```bash
kustomize build config/default | kubectl apply -f -
```

### Method 4: Using Release Manifests

For quick deployment without Helm:
```bash
kubectl apply -f https://github.com/opzkit/database-user-operator/releases/latest/download/database-user-operator.yaml
```

## Configuration

### AWS Credentials Setup

The operator needs AWS credentials to create and manage secrets in AWS Secrets Manager.

**See [AWS Credentials](AWS_CREDENTIALS.md) for complete setup instructions** including:
- IRSA for EKS (recommended)
- Static credentials from Kubernetes Secrets
- EC2 instance profiles
- Required IAM permissions

### Resource Limits (Optional)

Recommended resource configuration:

```yaml
resources:
  limits:
    cpu: 500m
    memory: 256Mi
  requests:
    cpu: 10m
    memory: 64Mi
```

### Namespace Configuration

By default, the operator installs to `database-user-operator-system`. To change:

```bash
cd config/default && kustomize edit set namespace <your-namespace>
kustomize build config/default | kubectl apply -f -
```

## Post-Installation

### 1. Verify the Operator is Running

```bash
kubectl get pods -n database-user-operator-system
```

Expected output:
```
NAME                                                     READY   STATUS    RESTARTS   AGE
database-user-operator-controller-manager-xxxxx-xxxxx   2/2     Running   0          1m
```

### 2. Check CRD Installation

```bash
kubectl get crds | grep database.opzkit.io
```

Expected output:
```
databases.database.opzkit.io            2024-11-10T12:00:00Z
```

### 3. View Operator Logs

```bash
kubectl logs -n database-user-operator-system \
  deployment/database-user-operator-controller-manager \
  -c manager \
  --follow
```

### 4. Create a Test Database

Create a test Database resource to verify everything works:

```yaml
apiVersion: database.opzkit.io/v1alpha1
kind: Database
metadata:
  name: test-db
  namespace: default
spec:
  engine: postgres
  databaseName: test_database
  connectionStringSecretRef:
    name: postgres-admin
  awsSecretsManager:
    region: us-east-1
  retainOnDelete: false  # Cleanup after testing
```

Apply and check:
```bash
kubectl apply -f test-db.yaml
kubectl get database test-db
kubectl describe database test-db
```

Clean up:
```bash
kubectl delete database test-db
```

## Upgrading

### Using Helm

Upgrade to a new version:
```bash
helm upgrade database-user-operator ./helm/database-user-operator \
  --namespace db-system
```

**Note:** Helm automatically upgrades CRDs if they exist in the chart's `crds/` directory.

### Using Make

```bash
make deploy IMG=ghcr.io/opzkit/database-user-operator:v0.2.0
```

Then upgrade CRDs separately:
```bash
make install
```

### Using kubectl

```bash
kubectl set image deployment/database-user-operator-controller-manager \
  manager=ghcr.io/opzkit/database-user-operator:v0.2.0 \
  -n database-user-operator-system
```

And upgrade CRDs:
```bash
kubectl apply -f https://github.com/opzkit/database-user-operator/releases/download/v0.2.0/crds.yaml
```

## Uninstallation

### Using Helm

```bash
helm uninstall database-user-operator --namespace db-system
```

To also remove CRDs (**Warning**: This deletes all Database resources!):
```bash
kubectl delete crd databases.database.opzkit.io
```

### Using Make

```bash
make undeploy  # Remove operator
make uninstall # Remove CRDs (Warning: deletes all Database resources!)
```

### Using kubectl

```bash
kubectl delete namespace database-user-operator-system  # Remove operator
kubectl delete crd databases.database.opzkit.io        # Remove CRDs
```

### Clean Up Database Resources

If `retainOnDelete: true` (default), databases, users, and secrets remain after deleting the CRD. To clean up:

1. Set `retainOnDelete: false` on all Database resources
2. Delete the Database resources
3. Wait for cleanup to complete
4. Then uninstall the operator

## Troubleshooting Installation

### CRD Installation Fails

```bash
# Check if you have admin permissions
kubectl auth can-i create customresourcedefinitions

# Manually install CRDs
kubectl apply -f config/crd/bases/database.opzkit.io_databases.yaml
```

### Operator Pod Won't Start

Check logs:
```bash
kubectl logs -n database-user-operator-system \
  deployment/database-user-operator-controller-manager \
  -c manager
```

Common issues:
- Missing AWS credentials
- Invalid AWS permissions
- Network connectivity issues

### AWS Permission Issues

Verify the operator has correct AWS permissions:
```bash
# Check pod's AWS identity
kubectl exec -n database-user-operator-system \
  deployment/database-user-operator-controller-manager \
  -c manager -- \
  aws sts get-caller-identity
```

See [AWS Credentials](AWS_CREDENTIALS.md) for required IAM permissions.

## Next Steps

- [Usage Guide](USAGE.md) - Learn how to create and manage databases
- [Troubleshooting](TROUBLESHOOTING.md) - Common issues and solutions
- [AWS Credentials](AWS_CREDENTIALS.md) - IAM permissions and authentication
