# Development Guide

Guide for contributing to and developing the Database User Operator.

## Table of Contents
- [Development Setup](#development-setup)
- [Building](#building)
- [Testing](#testing)
- [Making Changes](#making-changes)
- [Code Structure](#code-structure)
- [Release Process](#release-process)

## Development Setup

### Prerequisites

- **Go 1.22+**: [Install Go](https://golang.org/doc/install)
- **Docker or Podman**: For building images
- **kubectl**: [Install kubectl](https://kubernetes.io/docs/tasks/tools/)
- **Kubernetes cluster**: Local (kind, minikube, k3d) or remote
- **make**: Usually pre-installed on Unix systems
- **kustomize**: Installed via `make kustomize`
- **controller-gen**: Installed via `make controller-gen`

### Clone the Repository

```bash
git clone https://github.com/opzkit/database-user-operator.git
cd database-user-operator
```

### Install Development Tools

```bash
# Install code generation tools
make controller-gen
make kustomize
make envtest

# Install pre-commit hooks
pip install pre-commit
pre-commit install
```

### Set Up Local Cluster

Using kind:
```bash
kind create cluster --name operator-dev
kubectl cluster-info --context kind-operator-dev
```

Using minikube:
```bash
minikube start --profile operator-dev
```

### Install CRDs

```bash
make install
```

## Building

### Build Binary

```bash
# Build for current platform
make build

# Run directly (without building image)
make run
```

### Build Docker Image

```bash
# Build image
make docker-build IMG=database-user-operator:dev

# Build and load into kind
make docker-build IMG=database-user-operator:dev
kind load docker-image database-user-operator:dev --name operator-dev
```

### Deploy to Cluster

```bash
# Deploy with dev image
make deploy IMG=database-user-operator:dev

# Check deployment
kubectl get pods -n database-user-operator-system
kubectl logs -n database-user-operator-system \
  deployment/database-user-operator-controller-manager \
  -c manager \
  --follow
```

### Deploy with Local Helm Chart

For testing Helm chart changes or local development:

```bash
# Ensure CRDs are included in the chart
make helm-crds

# Install from local chart
helm install database-user-operator ./helm/database-user-operator \
  --namespace db-system \
  --create-namespace \
  --set image.repository=database-user-operator \
  --set image.tag=dev \
  --set image.pullPolicy=Never

# Or install a specific version
helm install database-user-operator ./helm/database-user-operator \
  --namespace db-system \
  --create-namespace

# With custom values
helm install database-user-operator ./helm/database-user-operator \
  --namespace db-system \
  --create-namespace \
  --set replicaCount=2 \
  --set controllerManager.resources.limits.memory=256Mi
```

To upgrade after making changes:
```bash
# Regenerate CRDs if needed
make helm-crds

# Upgrade the release
helm upgrade database-user-operator ./helm/database-user-operator \
  --namespace db-system
```

To uninstall:
```bash
helm uninstall database-user-operator \
  --namespace db-system
```

## Testing

### Unit Tests

```bash
# Run all tests
make test

# Run with verbose output
go test -v ./...

# Run specific package
go test -v ./internal/controller

# Run with coverage
go test -cover ./...
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Integration Tests

```bash
# Install test dependencies
make envtest

# Run controller tests (requires test environment)
go test -v ./internal/controller/...
```

### Manual Testing

1. Create test PostgreSQL database:
```bash
# Using Docker
docker run --name postgres-test \
  -e POSTGRES_PASSWORD=testpass \
  -e POSTGRES_USER=admin \
  -p 5432:5432 \
  -d postgres:15

# Create connection secret
kubectl create secret generic postgres-admin \
  --from-literal=connectionString='postgresql://admin:testpass@host.docker.internal:5432/postgres?sslmode=disable'
```

2. Create test Database resource:
```yaml
apiVersion: database.opzkit.io/v1alpha1
kind: Database
metadata:
  name: test-db
spec:
  engine: postgres
  databaseName: test_database
  connectionStringSecretRef:
    name: postgres-admin
  retainOnDelete: false  # Cleanup after test
  awsSecretsManager:
    region: us-east-1
```

3. Apply and observe:
```bash
kubectl apply -f test-db.yaml
kubectl get database test-db -w
kubectl describe database test-db
```

4. Verify in PostgreSQL:
```bash
docker exec -it postgres-test psql -U admin -c "\l"  # List databases
docker exec -it postgres-test psql -U admin -c "\du" # List users
```

5. Verify in AWS:
```bash
aws secretsmanager list-secrets --filters Key=name,Values=test
aws secretsmanager get-secret-value --secret-id rds/postgres/test_database
```

6. Clean up:
```bash
kubectl delete database test-db
docker stop postgres-test
docker rm postgres-test
```

### Linting

```bash
# Run pre-commit checks
pre-commit run --all-files

# Run golangci-lint manually
golangci-lint run

# Format code
go fmt ./...
gofmt -s -w .
```

## Making Changes

### Development Workflow

1. **Create a feature branch**:
```bash
git checkout -b feature/my-new-feature
```

2. **Make changes** to the code

3. **Update generated code** (if you changed API types):
```bash
make generate  # Generate deepcopy, client code
make manifests # Generate CRD YAML
```

4. **Run tests**:
```bash
make test
```

5. **Build and test locally**:
```bash
make docker-build IMG=database-user-operator:dev
make deploy IMG=database-user-operator:dev
# Test your changes...
```

6. **Commit changes**:
```bash
git add .
git commit -m "feat: add new feature"
# pre-commit hooks run automatically
```

7. **Push and create PR**:
```bash
git push origin feature/my-new-feature
```

### Modifying CRD

If you change the CRD (`api/v1alpha1/database_types.go`):

1. Update the struct:
```go
type DatabaseSpec struct {
    // Add new field with kubebuilder markers
    // +optional
    // +kubebuilder:validation:MaxLength=100
    NewField string `json:"newField,omitempty"`
}
```

2. Regenerate manifests:
```bash
make generate
make manifests
```

3. Reinstall CRDs:
```bash
make install
```

4. Update documentation (samples, docs)

5. Add tests for new field

### Adding a New Controller

1. Create controller file in `internal/controller/`:
```go
package controller

type MyNewReconciler struct {
    client.Client
    Scheme *runtime.Scheme
}

func (r *MyNewReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Implementation
}
```

2. Register in `cmd/main.go`:
```go
if err = (&controller.MyNewReconciler{
    Client: mgr.GetClient(),
    Scheme: mgr.GetScheme(),
}).SetupWithManager(mgr); err != nil {
    setupLog.Error(err, "unable to create controller", "controller", "MyNew")
    os.Exit(1)
}
```

3. Add tests in `internal/controller/mynew_controller_test.go`

## Code Structure

```
.
├── api/
│   └── v1alpha1/
│       ├── database_types.go      # CRD definition
│       └── groupversion_info.go
├── cmd/
│   └── main.go                    # Operator entry point
├── config/
│   ├── crd/                       # CRD manifests
│   ├── default/                   # Kustomize default
│   ├── manager/                   # Deployment config
│   ├── rbac/                      # RBAC manifests
│   └── samples/                   # Example CRs
├── docs/                          # Documentation
├── internal/
│   ├── controller/
│   │   ├── database_controller.go        # Main reconciliation logic
│   │   ├── database_controller_test.go   # Controller tests
│   │   └── suite_test.go                 # Test suite setup
│   ├── database/
│   │   └── postgres.go                   # PostgreSQL operations
│   └── secrets/
│       └── aws_secrets_manager.go        # AWS Secrets Manager client
├── Makefile                       # Build automation
└── go.mod                         # Go dependencies
```

### Key Components

**API Types** (`api/v1alpha1/`):
- CRD definitions
- Validation rules (kubebuilder markers)
- defaulting logic

**Controller** (`internal/controller/`):
- Reconciliation loop
- Status management
- Finalizer logic
- Error handling

**Database Client** (`internal/database/`):
- PostgreSQL operations
- Connection management
- User/database/privilege management

**Secrets Client** (`internal/secrets/`):
- AWS Secrets Manager integration
- Secret CRUD operations
- Custom error types

## Release Process

### Version Tagging

Follow semantic versioning (vX.Y.Z):
- Major (X): Breaking changes
- Minor (Y): New features, backwards compatible
- Patch (Z): Bug fixes

### Creating a Release

1. **Update version** in relevant files:
```bash
# Update image tags in config/manager/kustomization.yaml
# Update CHANGELOG.md
```

2. **Commit version bump**:
```bash
git add .
git commit -m "chore: bump version to v0.2.0"
```

3. **Create and push tag**:
```bash
git tag v0.2.0
git push origin v0.2.0
```

4. **Build and push image**:
```bash
make docker-build docker-push IMG=ghcr.io/opzkit/database-user-operator:v0.2.0
```

5. **Create GitHub release**:
- Go to GitHub Releases
- Create new release from tag
- Add release notes from CHANGELOG.md
- Attach installation manifests

### Release Checklist

- [ ] All tests passing
- [ ] Documentation updated
- [ ] CHANGELOG.md updated
- [ ] Version bumped in all files
- [ ] Git tag created
- [ ] Docker image built and pushed
- [ ] GitHub release created
- [ ] Release notes published

## Contributing Guidelines

### Code Style

- Follow [Effective Go](https://golang.org/doc/effective_go)
- Use `gofmt` for formatting
- Run `golangci-lint` before committing
- Add comments for exported functions
- Use meaningful variable names

### Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add new feature
fix: resolve bug
docs: update documentation
test: add tests
chore: maintenance task
refactor: code refactoring
```

### Pull Request Process

1. Fork the repository
2. Create a feature branch
3. Make your changes with tests
4. Ensure all tests pass
5. Update documentation
6. Submit pull request with clear description
7. Address review feedback

### Testing Requirements

- Unit tests for new functions
- Integration tests for controller changes
- Manual testing in local cluster
- Documentation for new features

## Troubleshooting Development

### "make run" fails

```bash
# Ensure CRDs are installed
make install

# Check if another instance is running
ps aux | grep database-user-operator
```

### Generated files out of sync

```bash
# Regenerate everything
make generate
make manifests
```

### Test failures

```bash
# Clean and reinstall test environment
rm -rf bin/
make envtest
make test
```

### Docker build fails

```bash
# Clean docker cache
docker system prune -a

# Rebuild from scratch
make docker-build IMG=database-user-operator:dev --no-cache
```

## Resources

- [Kubebuilder Book](https://book.kubebuilder.io/)
- [Operator SDK](https://sdk.operatorframework.io/)
- [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime)
- [Kubernetes API Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md)

## Getting Help

- GitHub Issues: Report bugs or request features
- GitHub Discussions: Ask questions, share ideas
- Slack: Join #database-operator channel (if available)
