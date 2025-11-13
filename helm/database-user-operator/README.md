# Database User Operator Helm Chart

A Kubernetes operator for managing PostgreSQL and MySQL databases with AWS Secrets Manager integration.

## TL;DR

```bash
helm install database-user-operator ./helm/database-user-operator \
  --namespace db-system \
  --create-namespace
```

## Introduction

This chart installs the Database User Operator on a Kubernetes cluster using the Helm package manager. The operator automates:
- Database and user creation in PostgreSQL, MySQL, and MariaDB
- Secure password generation
- Credential storage in AWS Secrets Manager
- Customizable secret formats via templates

## Prerequisites

- Kubernetes 1.28+
- Helm 3.0+
- AWS credentials with Secrets Manager permissions (via IRSA, instance profile, or environment variables)

## Installing the Chart

### Basic Installation

```bash
helm install database-user-operator ./helm/database-user-operator \
  --namespace db-system \
  --create-namespace
```

### With Custom Values

```bash
helm install database-user-operator ./helm/database-user-operator \
  --namespace db-system \
  --create-namespace \
  --set replicaCount=2 \
  --set metrics.serviceMonitor.enabled=true
```

### With Values File

```bash
helm install database-user-operator ./helm/database-user-operator \
  --namespace db-system \
  --create-namespace \
  --values custom-values.yaml
```

## Uninstalling the Chart

```bash
helm uninstall database-user-operator --namespace db-system
```

This removes all Kubernetes components associated with the chart, but **does not** delete:
- Database resources (by default, `retainOnDelete: true`)
- CRDs (must be manually deleted)
- AWS Secrets Manager secrets (retained with 7-day recovery window by default)

## Configuration

### Operator Configuration

The following table lists the configurable parameters for the operator deployment.

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of operator replicas (leader election ensures only one active) | `1` |
| `image.repository` | Operator image repository | `ghcr.io/opzkit/database-user-operator` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `image.tag` | Image tag (defaults to chart appVersion) | `""` |
| `imagePullSecrets` | Image pull secrets | `[]` |
| `nameOverride` | Override chart name | `""` |
| `fullnameOverride` | Override full name | `""` |

#### Controller Manager

| Parameter | Description | Default |
|-----------|-------------|---------|
| `controllerManager.resources.limits.cpu` | CPU limit | `500m` |
| `controllerManager.resources.limits.memory` | Memory limit | `128Mi` |
| `controllerManager.resources.requests.cpu` | CPU request | `10m` |
| `controllerManager.resources.requests.memory` | Memory request | `64Mi` |
| `controllerManager.livenessProbe.initialDelaySeconds` | Liveness probe initial delay | `15` |
| `controllerManager.livenessProbe.periodSeconds` | Liveness probe period | `20` |
| `controllerManager.readinessProbe.initialDelaySeconds` | Readiness probe initial delay | `5` |
| `controllerManager.readinessProbe.periodSeconds` | Readiness probe period | `10` |

#### Kube-RBAC-Proxy Sidecar

| Parameter | Description | Default |
|-----------|-------------|---------|
| `kubeRbacProxy.enabled` | Enable kube-rbac-proxy sidecar for metrics | `true` |
| `kubeRbacProxy.image.repository` | kube-rbac-proxy image repository | `gcr.io/kubebuilder/kube-rbac-proxy` |
| `kubeRbacProxy.image.tag` | kube-rbac-proxy image tag | `v0.15.0` |
| `kubeRbacProxy.resources.limits.cpu` | CPU limit | `500m` |
| `kubeRbacProxy.resources.limits.memory` | Memory limit | `128Mi` |
| `kubeRbacProxy.resources.requests.cpu` | CPU request | `5m` |
| `kubeRbacProxy.resources.requests.memory` | Memory request | `64Mi` |

#### Security & RBAC

| Parameter | Description | Default |
|-----------|-------------|---------|
| `podSecurityContext.runAsNonRoot` | Run as non-root user | `true` |
| `serviceAccount.create` | Create service account | `true` |
| `serviceAccount.annotations` | Service account annotations | `{}` |
| `serviceAccount.name` | Service account name (generated if empty) | `""` |
| `rbac.create` | Create RBAC resources | `true` |

#### Metrics & Monitoring

| Parameter | Description | Default |
|-----------|-------------|---------|
| `metrics.enabled` | Enable metrics service | `true` |
| `metrics.port` | Metrics service port | `8443` |
| `metrics.serviceMonitor.enabled` | Create Prometheus ServiceMonitor | `false` |
| `metrics.serviceMonitor.interval` | Scrape interval | `30s` |
| `metrics.serviceMonitor.scrapeTimeout` | Scrape timeout | `10s` |
| `metrics.serviceMonitor.additionalLabels` | Additional labels for ServiceMonitor | `{}` |

#### Pod Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `podAnnotations` | Pod annotations | `{}` |
| `podLabels` | Pod labels | `{}` |
| `nodeSelector` | Node selector | `{}` |
| `tolerations` | Tolerations | `[]` |
| `affinity` | Affinity rules | `{}` |
| `env` | Environment variables | `[]` |
| `extraVolumes` | Additional volumes | `[]` |
| `extraVolumeMounts` | Additional volume mounts | `[]` |

### Database Resource Configuration

After installing the operator, create Database custom resources. See `values.yaml` for comprehensive examples of all available fields.

#### Minimal Example (PostgreSQL)

```yaml
apiVersion: database.opzkit.io/v1alpha1
kind: Database
metadata:
  name: myapp-db
spec:
  engine: postgres
  databaseName: myapp_db
  connectionStringSecretRef:
    name: postgres-admin
  awsSecretsManager:
    region: us-east-1
```

#### Minimal Example (MySQL)

```yaml
apiVersion: database.opzkit.io/v1alpha1
kind: Database
metadata:
  name: myapp-mysql-db
spec:
  engine: mysql
  databaseName: myapp_db
  connectionStringSecretRef:
    name: mysql-admin
  awsSecretsManager:
    region: us-east-1
```

For complete Database resource examples including custom secret templates, see the [project README](../../README.md#advanced-examples) and [Secret Templates documentation](../../docs/SECRET_TEMPLATES.md).

## AWS Credentials

The operator needs AWS credentials to access Secrets Manager. Configure using one of:

### IRSA (Recommended for EKS)

```yaml
serviceAccount:
  annotations:
    eks.amazonaws.com/role-arn: arn:aws:iam::123456789012:role/database-operator-role
```

### Environment Variables

```yaml
env:
  - name: AWS_REGION
    value: us-east-1
  - name: AWS_ACCESS_KEY_ID
    valueFrom:
      secretKeyRef:
        name: aws-credentials
        key: access-key-id
  - name: AWS_SECRET_ACCESS_KEY
    valueFrom:
      secretKeyRef:
        name: aws-credentials
        key: secret-access-key
```

### Instance Profile

For nodes with IAM instance profiles, no additional configuration is needed.

## High Availability

The operator supports multiple replicas with built-in leader election:

```yaml
replicaCount: 3
```

Only one replica actively reconciles resources at a time. Leader election ensures automatic failover.

## Prometheus Integration

Enable Prometheus monitoring:

```yaml
metrics:
  serviceMonitor:
    enabled: true
    interval: 30s
    additionalLabels:
      prometheus: kube-prometheus
```

## Upgrading

### From 0.x to 0.x

```bash
helm upgrade database-user-operator ./helm/database-user-operator \
  --namespace db-system
```

## Troubleshooting

### Check Operator Logs

```bash
kubectl logs -n db-system -l app.kubernetes.io/name=database-user-operator
```

### Check Database Resource Status

```bash
kubectl get databases -A
kubectl describe database <name>
```

### Common Issues

1. **AWS Permissions**: Ensure the operator has Secrets Manager permissions
2. **Connection Strings**: Verify admin connection string secrets exist
3. **CRD Version**: After upgrade, ensure CRDs are updated manually if needed

## Additional Documentation

- [Installation Guide](../../docs/INSTALLATION.md)
- [Usage Guide](../../docs/USAGE.md)
- [Secret Templates](../../docs/SECRET_TEMPLATES.md)
- [AWS Credentials Setup](../../docs/AWS_CREDENTIALS.md)
- [Troubleshooting](../../docs/TROUBLESHOOTING.md)

## Source Code

- https://github.com/opzkit/database-user-operator

## License

MIT License - see [LICENSE](../../LICENSE) for details
