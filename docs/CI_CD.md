# CI/CD Documentation

This document describes the Continuous Integration and Continuous Deployment (CI/CD) setup for the Database User Operator.

## Overview

The project uses GitHub Actions for CI/CD with two main workflows:

1. **Build and Test** (`build.yaml`) - Runs on every push and pull request
2. **Release** (`release.yaml`) - Runs on version tags to publish releases

## Build and Test Workflow

Location: `.github/workflows/build.yaml`

This workflow runs automatically on:
- Push to `main` or `develop` branches
- Pull requests targeting `main` or `develop` branches

### Jobs

#### 1. Lint

Runs `golangci-lint` to ensure code quality and style compliance.

**Steps:**
- Checkout code
- Set up Go 1.24
- Run golangci-lint

#### 2. Test

Runs unit tests with coverage reporting.

**Steps:**
- Checkout code
- Set up Go 1.24
- Download dependencies
- Generate code and manifests
- Run unit tests (`make test`)
- Upload coverage to Codecov

**Coverage:**
- Coverage report is uploaded to Codecov with the `unittests` flag
- Report is available at `cover.out`

#### 3. Build

Builds the operator binary and Docker image to verify compilation.

**Steps:**
- Checkout code
- Set up Go 1.24
- Download dependencies
- Generate code and manifests
- Build binary (`make build`)
- Build Docker image

**Output:**
- Binary at `bin/manager`
- Docker image tagged with commit SHA

#### 4. Verify Manifests

Ensures generated files (CRDs, RBAC, etc.) are up to date.

**Steps:**
- Checkout code
- Set up Go 1.24
- Generate manifests and code
- Verify no uncommitted changes

**Failure Condition:**
If generated files don't match committed files, the job fails with instructions to run `make manifests generate`.

#### 5. Integration Tests

Runs comprehensive integration tests against a real Kubernetes cluster with PostgreSQL, MySQL, and LocalStack.

**Steps:**
1. **Setup:**
   - Checkout code
   - Set up Go 1.24
   - Download dependencies
   - Install tools (kind, kubectl, helm)

2. **Cluster Setup:**
   - Create kind cluster with databases
   - Deploy operator with coverage instrumentation
   - Verify cluster is ready

3. **Testing:**
   - Run integration tests (`make integration-run`)
   - Tests run against real database instances
   - Coverage data is collected from running operator

4. **Reporting:**
   - Merge unit and integration coverage data
   - Upload merged coverage to Codecov with `integration` flag
   - Generate HTML coverage report

5. **Debug on Failure:**
   - Dump cluster information
   - Export operator logs (last 500 lines)
   - Show database pod status
   - Display recent cluster events
   - Export complete kind logs

6. **Cleanup (Always Runs):**
   - Teardown kind cluster
   - Cleanup resources
   - Ensures no cluster remnants remain

**Timeout:** 30 minutes

**Artifacts on Failure:**
- Kind cluster logs (7 day retention)

**Coverage:**
- Merged coverage from unit and integration tests
- Available at `coverage/merged.out`
- HTML report at `coverage/coverage.html`

## Release Workflow

Location: `.github/workflows/release.yaml`

This workflow runs when a version tag is pushed (e.g., `v1.0.0`).

### Jobs

#### 1. GoReleaser

Creates GitHub release with binaries for multiple platforms.

**Steps:**
- Checkout code with full history
- Set up Go 1.24
- Generate code and manifests
- Run GoReleaser to build and publish

**Outputs:**
- GitHub release with changelog
- Multi-platform binaries

#### 2. Docker

Builds and pushes multi-architecture Docker images.

**Steps:**
- Set up QEMU for cross-platform builds
- Set up Docker Buildx
- Login to GitHub Container Registry (ghcr.io)
- Build and push images for linux/amd64 and linux/arm64

**Tags Generated:**
- `ghcr.io/OWNER/REPO:v1.0.0` (semver)
- `ghcr.io/OWNER/REPO:v1.0` (major.minor)
- `ghcr.io/OWNER/REPO:v1` (major)
- `ghcr.io/OWNER/REPO:sha-COMMIT` (commit SHA)

**Platforms:**
- linux/amd64
- linux/arm64

#### 3. Release Manifests

Generates Kubernetes manifests for the release.

**Steps:**
- Generate CRDs and RBAC
- Build release manifests with Kustomize
- Upload manifests to GitHub release

**Output:**
- `database-user-operator.yaml` - Complete installation manifest

## Coverage Reporting

The project uses Codecov for coverage tracking with two separate flags:

### Unit Test Coverage
- Flag: `unittests`
- Source: Unit tests via `make test`
- File: `cover.out`

### Integration Test Coverage
- Flag: `integration`
- Source: Integration tests + unit tests merged
- File: `coverage/merged.out`

### Coverage Collection Process

1. **Unit Tests:**
   - Run with `-coverprofile cover.out`
   - Standard Go coverage instrumentation

2. **Integration Tests:**
   - Operator built with `-cover` flag
   - `GOCOVERDIR=/tmp/coverage` environment variable
   - Coverage data written during operator execution
   - Merged with unit test coverage

3. **Merged Report:**
   - Created by `make merge-coverage`
   - Combines unit and integration coverage
   - Provides comprehensive view of tested code

## Local Testing

### Run All Checks Locally

```bash
# Lint
make lint

# Unit tests
make test

# Build
make build

# Integration tests
make integration-test
```

### Test Individual Jobs

```bash
# Run only linting
make lint

# Run only unit tests
make test

# Run only integration tests (full workflow)
make integration-test

# Or run integration tests step by step:
make integration-setup
make integration-run
make integration-teardown
```

## Troubleshooting CI Failures

### Lint Failures

Run locally to see and fix issues:
```bash
make lint
make lint-fix  # Auto-fix some issues
```

### Test Failures

Run locally with verbose output:
```bash
make test
go test -v ./...
```

### Integration Test Failures

Check the workflow logs for:
1. Cluster setup issues
2. Operator logs (last 500 lines)
3. Database pod status
4. Recent cluster events

Download kind logs artifact from failed run for detailed debugging.

Run locally with debugging:
```bash
make integration-setup
# Inspect cluster
kubectl get pods -A
kubectl logs -n db-system deployment/database-user-operator
# Run tests
make integration-run
# Don't forget cleanup
make integration-teardown
```

### Manifest Verification Failures

If generated files are out of date:
```bash
make manifests generate
git diff  # Review changes
git add .
git commit -m "chore: update generated files"
```

## Required Secrets

### Codecov (Optional)

If using Codecov token:
- `CODECOV_TOKEN` - Repository upload token
- Set in repository secrets at `Settings > Secrets and variables > Actions`

Note: Public repositories don't require a token.

### GitHub Container Registry

No additional secrets needed - uses `GITHUB_TOKEN` automatically provided by GitHub Actions.

## Best Practices

1. **Always run tests locally** before pushing
2. **Keep generated files in sync** by running `make manifests generate`
3. **Review coverage reports** to maintain test quality
4. **Monitor integration test duration** (should stay under 30 minutes)
5. **Use feature branches** for development
6. **Ensure PRs pass all checks** before merging

## Workflow Triggers

### Build and Test

```yaml
on:
  push:
    branches:
      - main
      - develop
  pull_request:
    branches:
      - main
      - develop
```

### Release

```yaml
on:
  push:
    tags:
      - 'v*'
```

## Performance Considerations

### Caching

All workflows use Go module caching to speed up builds:
```yaml
- uses: actions/setup-go@v5
  with:
    cache: true
```

### Integration Test Optimization

- Tests run in parallel where possible
- Kind cluster uses local registry caching
- Docker build layers are cached
- Coverage collection is optimized to minimize overhead

## Future Improvements

Potential enhancements to consider:

1. **Matrix Testing:** Test against multiple Kubernetes versions
2. **Database Versions:** Test against different PostgreSQL/MySQL versions
3. **Parallel Integration Tests:** Split tests into multiple jobs
4. **Performance Testing:** Add benchmarking jobs
5. **Security Scanning:** Add container vulnerability scanning
6. **End-to-End Tests:** Add E2E tests with real AWS Secrets Manager

## References

- [GitHub Actions Documentation](https://docs.github.com/en/actions)
- [Kind Documentation](https://kind.sigs.k8s.io/)
- [Codecov Documentation](https://docs.codecov.com/)
- [GoReleaser Documentation](https://goreleaser.com/)
