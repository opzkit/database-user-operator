# Contributing to Database User Operator

Thank you for your interest in contributing to the Database User Operator!

## Development Setup

### Prerequisites

- Go 1.22 or later
- Docker or Podman
- kubectl
- A Kubernetes cluster (kind, minikube, or similar for local development)
- Git

### Getting Started

1. Fork and clone the repository:
```bash
git clone https://github.com/your-username/database-user-operator.git
cd database-user-operator
```

2. Install dependencies:
```bash
go mod download
```

3. Install development tools:
```bash
make controller-gen
make envtest
make kustomize
```

4. Set up pre-commit hooks (optional but recommended):
```bash
pip install pre-commit
pre-commit install
```

## Project Structure

```
database-user-operator/
├── api/v1alpha1/           # API definitions (CRDs)
├── cmd/manager/            # Main entry point
├── config/                 # Kubernetes manifests and Kustomize configs
├── internal/
│   ├── controller/        # Reconciliation logic
│   ├── database/          # Database client implementations (MySQL, PostgreSQL)
│   └── secrets/           # AWS Secrets Manager integration
├── helm/                  # Helm chart
├── test/integration/      # Integration tests
└── docs/                  # Documentation
```

## Development Workflow

### Making Changes

1. Create a new branch:
```bash
git checkout -b feature/your-feature-name
```

2. Make your changes

3. Generate code and manifests if you modified API types:
```bash
make generate
make manifests
```

4. Format your code:
```bash
make fmt
```

5. Run linters:
```bash
make lint
```

### Code Style

- Follow standard Go conventions
- Use `gofmt` for formatting (run via `make fmt`)
- Run `golangci-lint` before submitting (run via `make lint`)
- Add appropriate comments for exported functions and types
- Keep functions small and focused
- Write descriptive variable and function names

### Testing

The project uses both unit tests and integration tests:

#### Unit Tests

Run unit tests with coverage:
```bash
make test
```

This will:
- Generate required manifests
- Format code
- Run `go vet`
- Execute unit tests with coverage reporting

View coverage report:
```bash
go tool cover -html=cover.out
```

#### Integration Tests

Integration tests run against a real Kubernetes cluster with PostgreSQL, MySQL, and LocalStack (for AWS Secrets Manager emulation).

Run the complete integration test suite:
```bash
make integration-test
```

This will:
1. Run unit tests
2. Create a kind cluster with databases
3. Deploy the operator with coverage instrumentation
4. Run integration tests
5. Collect and merge coverage data
6. Clean up the cluster

For more details, see [test/integration/README.md](../test/integration/README.md).

#### Development Workflow for Integration Tests

During development, you can keep the cluster running:

```bash
# Setup once
make integration-setup

# Make changes to code
# ...

# Rebuild and redeploy operator (much faster than full setup)
make integration-rebuild

# Run tests
make integration-run

# When done
make integration-teardown
```

#### Coverage

- Write unit tests for new functionality
- Ensure test coverage doesn't decrease
- Integration tests automatically collect coverage when run via `make integration-test`
- Merged coverage report is generated in `coverage/merged.out`

### Running Locally

To run the operator locally against your Kubernetes cluster:

```bash
# Install CRDs
make install

# Run the operator (uses your current kubeconfig context)
make run
```

### Building

Build the manager binary:
```bash
make build
```

Build the Docker image:
```bash
make docker-build IMG=your-registry/database-user-operator:your-tag
```

Build multi-platform images:
```bash
make docker-buildx IMG=your-registry/database-user-operator:your-tag
```

## Submitting Changes

### Commit Messages

Follow conventional commit format:

- `feat:` - New features
- `fix:` - Bug fixes
- `docs:` - Documentation changes
- `test:` - Test additions or changes
- `refactor:` - Code refactoring
- `chore:` - Build process or auxiliary tool changes
- `perf:` - Performance improvements

Example:
```
feat: add MySQL database support

Add support for creating and managing MySQL databases and users
with configurable privileges and character set options.
```

### Pull Request Process

1. Update documentation if needed
2. Ensure all tests pass (`make test` and `make integration-test`)
3. Ensure linters pass (`make lint`)
4. Update the README.md if adding new features
5. Run `make generate manifests` if you modified API types
6. Create a pull request with a clear description of changes
7. Link any related issues

### Pull Request Checklist

- [ ] Tests added/updated
- [ ] Documentation updated
- [ ] Commits follow conventional commit format
- [ ] All CI checks passing
- [ ] Generated files updated (`make generate manifests`)
- [ ] Code formatted (`make fmt`)
- [ ] Linters passing (`make lint`)
- [ ] Integration tests pass (if applicable)

## Code Review

All submissions require review. We use GitHub pull requests for this purpose.

### Review Process

1. A maintainer will review your PR
2. Address any feedback or requested changes
3. Once approved, a maintainer will merge your PR

### Review Guidelines

- Be respectful and constructive
- Focus on the code, not the person
- Provide specific, actionable feedback
- Acknowledge good work and improvements

## Reporting Issues

### Bug Reports

When reporting bugs, please include:
- Clear description of the issue
- Steps to reproduce
- Expected vs actual behavior
- Kubernetes version
- Operator version
- Database type and version
- Relevant logs (operator logs, kubectl describe output)

Example:
```bash
# Get operator logs
kubectl logs -n db-system deployment/database-user-operator

# Get resource status
kubectl describe database my-database
```

### Feature Requests

When requesting features, please include:
- Clear description of the proposed feature
- Use case and motivation
- Possible implementation approach
- Examples of how it would be used

## Development Tips

### Debugging

Enable verbose logging when running locally:
```bash
go run ./cmd/manager/main.go --zap-log-level=debug
```

View operator logs in cluster:
```bash
kubectl logs -n db-system deployment/database-user-operator -f
```

Check CRD status:
```bash
kubectl get database -A
kubectl describe database <name>
```

### Adding a New Database Engine

To add support for a new database engine:

1. Implement the `DatabaseClient` interface in `internal/database/`:
   - Create a new file (e.g., `mssql.go`)
   - Implement all required methods
   - Add factory logic in `factory.go`

2. Update API types if needed in `api/v1alpha1/database_types.go`

3. Run code generation:
   ```bash
   make generate manifests
   ```

4. Add unit tests and integration tests

5. Update documentation

### Modifying the API

When modifying CRD fields:

1. Edit `api/v1alpha1/database_types.go`
2. Add kubebuilder markers for validation, defaults, etc.
3. Generate code and manifests:
   ```bash
   make generate manifests
   ```
4. Update Helm chart CRDs:
   ```bash
   make helm-crds
   ```
5. Update documentation and examples

### Working with Helm Chart

Lint the chart:
```bash
make helm-lint
```

Test template rendering:
```bash
make helm-template
```

Package the chart:
```bash
make helm-package
```

## Community

- Be respectful and inclusive
- Follow the [Code of Conduct](https://www.contributor-covenant.org/version/2/1/code_of_conduct/)
- Help others in discussions and issues
- Welcome newcomers and first-time contributors

## Questions?

Feel free to open an issue for questions or reach out to maintainers.

## Additional Resources

- [CI/CD Documentation](CI_CD.md) - GitHub Actions workflows and coverage reporting
- [Kubebuilder Book](https://book.kubebuilder.io/) - Learn about operator development
- [Kubernetes API Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md)
- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)

Thank you for contributing!
